package herdrtest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/CLBRITTON2/calmpane/internal/herdr"
)

var fakeAPICount atomic.Int64

type fakeAPIRequest struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params struct {
		Target string `json:"target"`
		PaneID string `json:"pane_id"`
		Text   string `json:"text"`
	} `json:"params"`
}

// FakeAPI serves herdr's socket API on a pipe of its own, logging each request as "<method> <pane> <text>". It refuses
// agent.prompt to a pane whose AgentStatus is blocked, and to a pane not in panes.
func FakeAPI(t *testing.T, panes []herdr.Pane) (herdr.API, func() []string) {
	api := herdr.API{SocketPath: fmt.Sprintf("calmpane-test-%d-%d", os.Getpid(), fakeAPICount.Add(1))}
	pipeName := `\\.\pipe\` + api.SocketPath
	pipeNameUTF16, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		t.Fatal(err)
	}
	newPipeInstance := func() (windows.Handle, error) {
		return windows.CreateNamedPipe(pipeNameUTF16, windows.PIPE_ACCESS_DUPLEX,
			windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT, windows.PIPE_UNLIMITED_INSTANCES,
			1<<20, 1<<20, 0, nil)
	}
	pipe, err := newPipeInstance()
	if err != nil {
		t.Fatalf("create pipe %s: %v", pipeName, err)
	}
	var requestsLock sync.Mutex
	requests := []string{}
	logRequest := func(request string) {
		requestsLock.Lock()
		defer requestsLock.Unlock()
		requests = append(requests, request)
	}
	stopping := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			err := windows.ConnectNamedPipe(pipe, nil)
			if err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
				t.Errorf("connect pipe %s: %v", pipeName, err)
				return
			}
			select {
			case <-stopping:
				if err := windows.CloseHandle(pipe); err != nil {
					t.Error(err)
				}
				return
			default:
			}
			// The next instance exists before this one replies, so a client's next request finds it.
			nextPipe, err := newPipeInstance()
			if err != nil {
				t.Errorf("create pipe %s: %v", pipeName, err)
				return
			}
			if err := answerFakeAPIRequest(pipe, panes, logRequest); err != nil {
				t.Errorf("answer on pipe %s: %v", pipeName, err)
			}
			pipe = nextPipe
		}
	}()
	t.Cleanup(func() {
		close(stopping)
		client, err := os.OpenFile(pipeName, os.O_RDWR, 0)
		if err != nil {
			t.Errorf("open pipe %s to stop its server: %v", pipeName, err)
			return
		}
		if err := client.Close(); err != nil {
			t.Error(err)
		}
		<-stopped
	})
	return api, func() []string {
		requestsLock.Lock()
		defer requestsLock.Unlock()
		return append([]string{}, requests...)
	}
}

// answerFakeAPIRequest logs the request before replying, so the client's call returns with it logged. It waits for the
// client to read the reply before closing pipe, which would discard it.
func answerFakeAPIRequest(pipe windows.Handle, panes []herdr.Pane, logRequest func(request string)) error {
	connection := os.NewFile(uintptr(pipe), "herdr socket")
	requestLine, err := bufio.NewReader(connection).ReadBytes('\n')
	if err != nil {
		return errors.Join(err, connection.Close())
	}
	var request fakeAPIRequest
	if err := json.Unmarshal(requestLine, &request); err != nil {
		return errors.Join(err, connection.Close())
	}
	reply := fmt.Sprintf(`{"id":%q,"result":{}}`, request.ID)
	if request.Method == "agent.prompt" && !slices.ContainsFunc(panes, func(pane herdr.Pane) bool {
		return pane.PaneID == request.Params.Target
	}) {
		reply = fmt.Sprintf(`{"id":%q,"error":{"code":"pane_not_found","message":"pane %s not found"}}`, request.ID,
			request.Params.Target)
	}
	for _, pane := range panes {
		if request.Method == "agent.prompt" && pane.PaneID == request.Params.Target && pane.AgentStatus == "blocked" {
			reply = fmt.Sprintf(`{"id":%q,"error":{"code":"agent_blocked","message":"agent is blocked"}}`, request.ID)
		}
	}
	logRequest(request.Method + " " + request.Params.Target + request.Params.PaneID + " " + request.Params.Text)
	_, err = connection.WriteString(reply + "\n")
	if err == nil {
		err = windows.FlushFileBuffers(pipe)
	}
	return errors.Join(err, connection.Close())
}
