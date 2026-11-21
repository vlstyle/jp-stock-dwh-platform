package heartbeat

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"testing"
)

func fakeServer(t *testing.T, info string) (string, <-chan request) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	got := make(chan request, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		head := make([]byte, 13)
		if _, err := io.ReadFull(conn, head); err != nil {
			return
		}
		body := make([]byte, binary.LittleEndian.Uint64(head[5:]))
		if _, err := io.ReadFull(conn, body); err != nil {
			return
		}
		var r request
		_ = json.Unmarshal(body, &r)
		got <- r

		out, _ := json.Marshal(response{Response: "success", Info: info})
		msg := append([]byte{}, header...)
		msg = binary.LittleEndian.AppendUint64(msg, uint64(len(out)))
		_, _ = conn.Write(append(msg, out...))
	}()
	return ln.Addr().String(), got
}

func TestSend(t *testing.T) {
	addr, got := fakeServer(t, "processed: 1; failed: 0; total: 1; seconds spent: 0.000050")
	if err := Send(context.Background(), addr, "jp-stock-dwh", "prices", false); err != nil {
		t.Fatal(err)
	}
	r := <-got
	if r.Request != "sender data" || len(r.Data) != 1 {
		t.Fatalf("request = %+v", r)
	}
	if d := r.Data[0]; d.Host != "jp-stock-dwh" || d.Key != "cronjob.status[prices]" || d.Value != "0" {
		t.Errorf("data = %+v", d)
	}
}

func TestSendRejectedItem(t *testing.T) {
	addr, _ := fakeServer(t, "processed: 0; failed: 1; total: 1; seconds spent: 0.000050")
	if err := Send(context.Background(), addr, "jp-stock-dwh", "prices", true); err == nil {
		t.Fatal("expected error")
	}
}
