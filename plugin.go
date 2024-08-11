package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
    "html/template"

	"github.com/gotify/plugin-api"
    "github.com/gorilla/websocket"
)

// GetGotifyPluginInfo returns gotify plugin info
func GetGotifyPluginInfo() plugin.Info {
	return plugin.Info{
        Version: "1.1",
        Author: "Anh Bui & Leko",
		Name: "Gotify 2 Telegram",
        Description: "Telegram message fowarder for gotify",
		ModulePath: "https://github.com/lekoOwO/gotify2telegram",
	}
}

// Plugin is the plugin instance
type Plugin struct {
    config      *Config
    ws          map[int]*websocket.Conn
    msgHandler  plugin.MessageHandler
}

type GotifyMessage struct {
    Id          uint32
    Appid       uint32
    Message     string
    Title       string
    Priority    uint32
    Date        string
}

type Payload struct {
	ChatID   string `json:"chat_id"`
	Text     string `json:"text"`
    ThreadId string `json:"msg_thread_id"`
}

func (p *Plugin) send_msg_to_telegram(msg string, bot_token string, chat_id string, thread_id string) {
    step_size := 4090
    sending_message := ""
    for i:=0; i<len(msg); i+=step_size {
        if i+step_size < len(msg) {
			sending_message = msg[i : i+step_size]
		} else {
			sending_message = msg[i:len(msg)]
		}

        data := Payload{
            ChatID: chat_id,
            Text: sending_message,
            ThreadId: thread_id,
        }
        payloadBytes, err := json.Marshal(data)
        if err != nil {
            fmt.Println("Create json false")
            return
        }
        body := bytes.NewReader(payloadBytes)
        
        req, err := http.NewRequest("POST", "https://api.telegram.org/bot"+ bot_token +"/sendMessage", body)
        if err != nil {
            fmt.Println("Create request false")
            return
        }
        req.Header.Set("Content-Type", "application/json")
        
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            fmt.Printf("Send request false: %v\n", err)
            return
        }
        defer resp.Body.Close()
    }
}

func (p *Plugin) connect_websocket(url string, subClientIndex int) {
    for {
        ws, _, err := websocket.DefaultDialer.Dial(p.config.GotifyHost, nil)
        if err == nil {
            p.ws[subClientIndex] = ws
            break
        }
        fmt.Printf("Cannot connect to websocket: %v\n", err)
        time.Sleep(5)
    }
}

func format_telegram_message(msg *GotifyMessage) string {
    // HTML Should be escaped here
    title := string(template.HTML("<b>" + template.HTMLEscapeString(msg.Title) + "</b>"))
    return fmt.Sprintf(
        "%s\n%s\n\nDate: %s", 
        title, 
        template.HTMLEscapeString(msg.Message),
        msg.Date,
    )
}

func (p *Plugin) get_websocket_msg(url string, subClient *SubClient, subClientIndex int) {
    ws_url := url + "/stream?token=" + subClient.GotifyClientToken
    go p.connect_websocket(ws_url, subClientIndex)

    for {
        msg := &GotifyMessage{}
        if p.ws[subClientIndex] == nil {
            time.Sleep(3)
            continue
        }
        err := p.ws[subClientIndex].ReadJSON(msg)
        if err != nil {
            fmt.Printf("Error while reading websocket: %v\n", err)
            p.connect_websocket(ws_url, subClientIndex)
            continue
        }
        p.send_msg_to_telegram(
            format_telegram_message(msg),
            subClient.Telegram.BotToken,
            subClient.Telegram.ChatId,
            subClient.Telegram.ThreadId,
        )
    }
}

// SetMessageHandler implements plugin.Messenger
// Invoked during initialization
func (p *Plugin) SetMessageHandler(h plugin.MessageHandler) {
    p.msgHandler = h
}

func (p *Plugin) Enable() error {
    for i, subClient := range p.config.Clients {
        go p.get_websocket_msg(
            p.config.GotifyHost, 
            &subClient, i,
        )
    }
    return nil
}

// Disable implements plugin.Plugin
func (p *Plugin) Disable() error {
    for _, ws := range p.ws {
        if ws != nil {
            ws.Close()
        }
    }
    return nil
}

// NewGotifyPluginInstance creates a plugin instance for a user context.
func NewGotifyPluginInstance(ctx plugin.UserContext) plugin.Plugin {
    return &Plugin{
        ws: make(map[int]*websocket.Conn),
    }
}

func main() {
    panic("this should be built as go plugin")
}