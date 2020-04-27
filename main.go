package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
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

func (t *telegram) sendMessage(chatId string, text string, replyTo int) (int, error) {
	v := url.Values{}
	v.Set("chat_id", chatId)
	v.Set("text", text)
	v.Set("parse_mode", "MarkdownV2")
	if replyTo >= 0 {
		v.Set("reply_to_message_id", fmt.Sprintf("%d", replyTo))
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

	flag.StringVar(&id, "id", "", "notification identifier (e.g. machine hostname)")
	flag.BoolVar(&onSuccess, "success", false, "send notification if command executed successfully")
	flag.IntVar(&limit, "limit", 1024, "limit size of stream data sent in notification")
	flag.Parse()

	args := flag.Args()

	status, stdout, stderr, cmdErr := runCommand(args)
	if cmdErr == nil && status == 0 && !onSuccess {
		return
	}

	token, ok := os.LookupEnv("TELEGRAM_TOKEN")
	if !ok {
		log.Fatal("error: telegram token not defined")
	}

	chatId, ok := os.LookupEnv("TELEGRAM_CHAT_ID")
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
		msg += fmt.Sprintf("*%s:* ", id)
	}

	if cmdErr != nil {
		msg += "Command error\n\n"
		msg += fmt.Sprintf("*Command:* %q\n", args)
		msg += fmt.Sprintf("```\n%s```\n", cmdErr)
		if _, err := t.sendMessage(chatId, msg, -1); err != nil {
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
	msg += fmt.Sprintf("*Command:* `%q`\n", args)
	msg += fmt.Sprintf("*Exit status:* %d\n", status)
	if len(streamNames) > 0 {
		msg += fmt.Sprintf("*Streams:* %s\n", strings.Join(streamNames, ", "))
	}

	msgId, err := t.sendMessage(chatId, msg, -1)
	if err != nil {
		log.Fatal(err)
	}

	for i, stream := range streams {
		msg := fmt.Sprintf("*%s:*\n", streamNames[i])
		if len(stream) > limit {
			msg += fmt.Sprintf("```\n...%s```", stream[len(stream)-limit:])
		} else {
			msg += fmt.Sprintf("```\n%s```", stream)
		}
		if _, err := t.sendMessage(chatId, msg, msgId); err != nil {
			log.Fatal(err)
		}
	}
}
