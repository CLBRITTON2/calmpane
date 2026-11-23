package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// API is herdr's socket API, for the requests whose text can pass Windows' 32,767-character command line, which the
// CLI takes it on.
type API struct {
	// SocketPath is HERDR_SOCKET_PATH, which on Windows names the server's pipe \\.\pipe\<SocketPath>.
	SocketPath string
}

type agentPromptParams struct {
	Target string `json:"target"`
	Text   string `json:"text"`
}

type paneSendTextParams struct {
	PaneID string `json:"pane_id"`
	Text   string `json:"text"`
}

type apiRequest[Params agentPromptParams | paneSendTextParams] struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params Params `json:"params"`
}

type apiReply struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// PromptAgent returns once herdr has written the prompt and its Enter, and ErrAgentBlocked while the agent waits for an
// answer.
func (a API) PromptAgent(paneID string, prompt string) error {
	return send(a, apiRequest[agentPromptParams]{ID: "calmpane", Method: "agent.prompt",
		Params: agentPromptParams{Target: paneID, Text: prompt}})
}

func (a API) SendText(paneID string, text string) error {
	return send(a, apiRequest[paneSendTextParams]{ID: "calmpane", Method: "pane.send_text",
		Params: paneSendTextParams{PaneID: paneID, Text: text}})
}

// send writes the request as one JSON line and reads herdr's one-line reply. A failure names only the method, as the
// zen view's status line shows it: the params hold the whole prompt.
func send[Params agentPromptParams | paneSendTextParams](a API, request apiRequest[Params]) error {
	if a.SocketPath == "" {
		return fmt.Errorf("herdr %s failed: HERDR_SOCKET_PATH is not set", request.Method)
	}
	pipeName := `\\.\pipe\` + a.SocketPath
	requestLine, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("herdr %s failed: encode the request: %w", request.Method, err)
	}
	pipe, err := os.OpenFile(pipeName, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("herdr %s failed: %w", request.Method, err)
	}
	_, err = pipe.Write(append(requestLine, '\n'))
	var replyLine []byte
	if err == nil {
		replyLine, err = bufio.NewReader(pipe).ReadBytes('\n')
	}
	if err := errors.Join(err, pipe.Close()); err != nil {
		return fmt.Errorf("herdr %s failed: %w", request.Method, err)
	}
	var reply apiReply
	if err := json.Unmarshal(replyLine, &reply); err != nil {
		return fmt.Errorf("herdr %s failed: decode the reply %q: %w", request.Method, replyLine, err)
	}
	if reply.Error == nil {
		return nil
	}
	if reply.Error.Code == ErrAgentBlocked.Error() {
		return ErrAgentBlocked
	}
	return fmt.Errorf("herdr %s failed: %s: %s", request.Method, reply.Error.Code, reply.Error.Message)
}
