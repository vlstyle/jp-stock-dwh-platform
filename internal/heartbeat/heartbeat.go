// Package heartbeat は、ジョブの成否を Zabbix トラッパーに送信する。
package heartbeat

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const timeout = 10 * time.Second

var header = []byte("ZBXD\x01")

type item struct {
	Host  string `json:"host"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type request struct {
	Request string `json:"request"`
	Data    []item `json:"data"`
}

type response struct {
	Response string `json:"response"`
	Info     string `json:"info"`
}

// Send は、Zabbix サーバー(addr: host:port)のトラッパーアイテム cronjob.status[job] に、成功なら1、失敗なら0を送る。
func Send(ctx context.Context, addr, host, job string, ok bool) error {
	value := "0"
	if ok {
		value = "1"
	}
	body, err := json.Marshal(request{
		Request: "sender data",
		Data:    []item{{Host: host, Key: fmt.Sprintf("cronjob.status[%s]", job), Value: value}},
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	msg := append([]byte{}, header...)
	msg = binary.LittleEndian.AppendUint64(msg, uint64(len(body)))
	msg = append(msg, body...)
	if _, err := conn.Write(msg); err != nil {
		return err
	}

	raw, err := io.ReadAll(conn)
	if err != nil {
		return err
	}
	if len(raw) < len(header)+8 {
		return errors.New("zabbix: short response")
	}
	var resp response
	if err := json.Unmarshal(raw[len(header)+8:], &resp); err != nil {
		return err
	}
	if resp.Response != "success" {
		return fmt.Errorf("zabbix: %s", resp.Response)
	}
	return checkProcessed(resp.Info)
}

// checkProcessed は、info の "processed: 1; failed: 0; ..." の failed が0であることを確認する。
func checkProcessed(info string) error {
	var processed, failed int
	if _, err := fmt.Sscanf(info, "processed: %d; failed: %d;", &processed, &failed); err != nil {
		return fmt.Errorf("zabbix: unexpected info %q", info)
	}
	if failed != 0 || processed == 0 {
		return fmt.Errorf("zabbix: item not accepted (%s)", info)
	}
	return nil
}
