package ndt7

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestUnitUploadSetWriteDeadlineFailure(t *testing.T) {
	expected := errors.New("mocked error")
	mgr := newUploadManager(
		&mockableConnMock{
			WriteDeadlineErr: expected,
		},
		defaultCallbackPerformance,
	)
	err := mgr.run(context.Background())
	if !errors.Is(err, expected) {
		t.Fatal("not the error we expected")
	}
}

func TestUnitUploadNewMessageFailure(t *testing.T) {
	expected := errors.New("mocked error")
	mgr := newUploadManager(
		&mockableConnMock{},
		defaultCallbackPerformance,
	)
	mgr.newMessage = func(int) (*websocket.PreparedMessage, error) {
		return nil, expected
	}
	err := mgr.run(context.Background())
	if !errors.Is(err, expected) {
		t.Fatal("not the error we expected")
	}
}

func TestUnitUploadWritePreparedMessageFailure(t *testing.T) {
	expected := errors.New("mocked error")
	mgr := newUploadManager(
		&mockableConnMock{
			WritePreparedMessageErr: expected,
		},
		defaultCallbackPerformance,
	)
	err := mgr.run(context.Background())
	if !errors.Is(err, expected) {
		t.Fatal("not the error we expected")
	}
}

func TestUnitUploadWritePreparedMessageSubsequentFailure(t *testing.T) {
	expected := errors.New("mocked error")
	mgr := newUploadManager(
		&mockableConnMock{},
		defaultCallbackPerformance,
	)
	var already bool
	mgr.newMessage = func(int) (*websocket.PreparedMessage, error) {
		if !already {
			already = true
			return new(websocket.PreparedMessage), nil
		}
		return nil, expected
	}
	err := mgr.run(context.Background())
	if !errors.Is(err, expected) {
		t.Fatal("not the error we expected")
	}
}

func TestUnitUploadLoop(t *testing.T) {
	mgr := newUploadManager(
		&mockableConnMock{},
		defaultCallbackPerformance,
	)
	mgr.newMessage = func(int) (*websocket.PreparedMessage, error) {
		return new(websocket.PreparedMessage), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := mgr.run(ctx)
	if err != nil {
		t.Fatal(err)
	}
}