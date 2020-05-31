package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type telegram struct {
	token    string
	userName string
}

func newTelegram(token string) (*telegram, error) {
	t := &telegram{token: token}

	type user struct {
		UserName string `json:"username"`
	}

	me := user{}
	if err := t.request("getMe", nil, &me); err != nil {
		return nil, err
	}

	if me.UserName == "" {
		return nil, fmt.Errorf("error: telegram: failed to find bot username")
	}

	t.userName = me.UserName
	return t, nil
}

func (t *telegram) request(method string, parameters url.Values, result interface{}) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", t.token, method)
	r, err := http.PostForm(url, parameters)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	type base struct {
		Ok          bool        `json:"ok"`
		Description string      `json:"description"`
		Result      interface{} `json:"result"`
	}

	m := base{Result: result}
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		return err
	}

	if !m.Ok {
		if m.Description != "" {
			return fmt.Errorf("error: telegram: request failed: %s", m.Description)
		}
		return fmt.Errorf("error: telegram: request failed")
	}

	return nil
}

func (t *telegram) sendMessage(chatId string, text string, disableNotification bool, replyToMessageId int) (int, error) {
	v := url.Values{}
	v.Set("chat_id", chatId)
	v.Set("text", text)
	v.Set("parse_mode", "HTML")
	if disableNotification {
		v.Set("disable_notification", "true")
	}
	if replyToMessageId >= 0 {
		v.Set("reply_to_message_id", fmt.Sprintf("%d", replyToMessageId))
	}

	type message struct {
		MessageId int `json:"message_id"`
	}

	m := message{}
	if err := t.request("sendMessage", v, &m); err != nil {
		return 0, err
	}

	return m.MessageId, nil
}

func runCommand(args []string) (int, []byte, []byte, error) {
	if len(args) == 0 {
		return 0, nil, nil, fmt.Errorf("error: exec: program not defined")
	}

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

	status := 0
	if err := cmd.Run(); err != nil {
		if msg, ok := err.(*exec.ExitError); ok {
			if ws, ok := msg.Sys().(syscall.WaitStatus); ok {
				status = ws.ExitStatus()
			}
		} else {
			return 0, nil, nil, err
		}
	}

	return status, stdout.Bytes(), stderr.Bytes(), nil
}

func main() {
	log.SetPrefix("telegram-notify: ")
	log.SetFlags(0)

	var (
		id        string
		onSuccess bool
		limit     int
	)

	flag.StringVar(&id, "id", "", "notification origin identifier (e.g. machine hostname)")
	flag.BoolVar(&onSuccess, "success", false, "send notification if command executed successfully")
	flag.IntVar(&limit, "limit", 1024, "limit size of stream data (in bytes) to send in notifications")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		return
	}

	start := time.Now()
	status, stdout, stderr, cmdErr := runCommand(args)
	elapsed := time.Since(start)
	if cmdErr == nil && status == 0 && !onSuccess {
		return
	}

	token, ok := os.LookupEnv("TELEGRAM_NOTIFY_TOKEN")
	if !ok {
		log.Fatal("error: telegram token not defined")
	}

	chatId, ok := os.LookupEnv("TELEGRAM_NOTIFY_CHAT_ID")
	if !ok {
		log.Fatal("error: telegram chat id not defined")
	}

	t, err := newTelegram(token)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("sending notification as %s", t.userName)

	msg := ""
	if id != "" {
		msg += fmt.Sprintf("<strong>%s:</strong> ", id)
	}

	if cmdErr != nil {
		msg += "Command error\n\n"
		msg += fmt.Sprintf("<strong>Command:</strong> %s\n", html.EscapeString(fmt.Sprintf("%q", args)))
		msg += fmt.Sprintf("<strong>Elapsed time:</strong> %s\n", html.EscapeString(elapsed.String()))
		msg += fmt.Sprintf("<pre>%s</pre>", html.EscapeString(cmdErr.Error()))
		if _, err := t.sendMessage(chatId, msg, false, -1); err != nil {
			log.Fatal(err)
		}
		log.Fatalf("error: failed to run program: %q: %s", args, cmdErr)
	}

	streams := [][]byte{}
	streamNames := []string{}
	if len(stdout) > 0 {
		streams = append(streams, stdout)
		streamNames = append(streamNames, "stdout")
	}
	if len(stderr) > 0 {
		streams = append(streams, stderr)
		streamNames = append(streamNames, "stderr")
	}

	if status == 0 {
		msg += "Success\n\n"
	} else {
		msg += "Failure\n\n"
	}
	msg += fmt.Sprintf("<strong>Command:</strong> <code>%s</code>\n", html.EscapeString(fmt.Sprintf("%q", args)))
	msg += fmt.Sprintf("<strong>Elapsed time:</strong> %s\n", html.EscapeString(elapsed.String()))
	msg += fmt.Sprintf("<strong>Exit status:</strong> %d\n", status)
	if len(streamNames) > 0 {
		msg += fmt.Sprintf("<strong>Streams:</strong> %s\n", strings.Join(streamNames, ", "))
	}

	msgId, err := t.sendMessage(chatId, msg, onSuccess && status == 0, -1)
	if err != nil {
		log.Fatal(err)
	}

	for i, stream := range streams {
		msg := fmt.Sprintf("<strong>%s:</strong>\n", streamNames[i])
		if len(stream) > limit {
			msg += fmt.Sprintf("<pre>...%s</pre>", html.EscapeString(string(stream[len(stream)-limit:])))
		} else {
			msg += fmt.Sprintf("<pre>%s</pre>", html.EscapeString(string(stream)))
		}
		if _, err := t.sendMessage(chatId, msg, onSuccess && status == 0, msgId); err != nil {
			log.Fatal(err)
		}
	}

	os.Exit(status)
}
