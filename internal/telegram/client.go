package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	token      string
	apiBase    string
	httpClient *http.Client
}

func New(token string) *Client {
	return &Client{
		token:      token,
		apiBase:    "https://api.telegram.org",
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) WithAPIBase(base string) *Client {
	c.apiBase = base
	return c
}

func (c *Client) WithHTTPClient(h *http.Client) *Client {
	c.httpClient = h
	return c
}

type Bot struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

type apiResponse struct {
	Ok          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  *struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

type APIError struct {
	Code        int
	Description string
	RetryAfter  int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram api error %d: %s", e.Code, e.Description)
}

// maxRetries bounds how many times a 429 (rate-limited) call is retried while
// honoring the server-provided retry_after.
const maxRetries = 3

func (c *Client) call(ctx context.Context, method string, params url.Values, body any) (json.RawMessage, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		raw, err := c.callOnce(ctx, method, params, body)
		if err == nil {
			return raw, nil
		}
		lastErr = err
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Code != http.StatusTooManyRequests || attempt == maxRetries {
			return nil, err
		}
		wait := time.Duration(apiErr.RetryAfter) * time.Second
		if wait <= 0 {
			wait = time.Second
		}
		if wait > 60*time.Second {
			wait = 60 * time.Second
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

func (c *Client) callOnce(ctx context.Context, method string, params url.Values, body any) (json.RawMessage, error) {
	var resp *http.Response
	var err error
	if body != nil {
		data, merr := json.Marshal(body)
		if merr != nil {
			return nil, merr
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/bot"+c.token+"/"+method, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		resp, err = c.httpClient.Do(req)
	} else {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBase+"/bot"+c.token+"/"+method, strings.NewReader(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err = c.httpClient.Do(req)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var ar apiResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, fmt.Errorf("decoding telegram response: %w", err)
	}
	if !ar.Ok {
		retry := 0
		if ar.Parameters != nil {
			retry = ar.Parameters.RetryAfter
		}
		return nil, &APIError{Code: ar.ErrorCode, Description: ar.Description, RetryAfter: retry}
	}
	return ar.Result, nil
}

func (c *Client) GetMe(ctx context.Context) (Bot, error) {
	raw, err := c.call(ctx, "getMe", nil, nil)
	if err != nil {
		return Bot{}, err
	}
	var b Bot
	if err := json.Unmarshal(raw, &b); err != nil {
		return Bot{}, err
	}
	return b, nil
}

type SendMessageRequest struct {
	ChatID      string          `json:"chat_id"`
	Text        string          `json:"text"`
	ParseMode   string          `json:"parse_mode,omitempty"`
	ReplyMarkup json.RawMessage `json:"reply_markup,omitempty"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	return c.SendMessageWithMarkup(ctx, chatID, text, nil)
}

func (c *Client) SendMessageWithMarkup(ctx context.Context, chatID, text string, markup *InlineKeyboardMarkup) error {
	body := SendMessageRequest{ChatID: chatID, Text: text}
	if markup != nil {
		if data, err := json.Marshal(markup); err == nil {
			body.ReplyMarkup = data
		}
	}
	_, err := c.call(ctx, "sendMessage", nil, body)
	return err
}

func (c *Client) Token() string { return c.token }

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
	Date      int64  `json:"date"`
}

type CallbackQuery struct {
	ID      string  `json:"id"`
	From    User    `json:"from"`
	Message Message `json:"message"`
	Data    string  `json:"data"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	params := url.Values{}
	params.Set("offset", strconv.FormatInt(offset, 10))
	params.Set("timeout", strconv.Itoa(timeout))
	raw, err := c.call(ctx, "getUpdates", params, nil)
	if err != nil {
		return nil, err
	}
	var updates []Update
	if err := json.Unmarshal(raw, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	params := url.Values{}
	params.Set("callback_query_id", id)
	params.Set("text", text)
	_, err := c.call(ctx, "answerCallbackQuery", params, nil)
	return err
}

func (c *Client) EditMessageText(ctx context.Context, chatID string, messageID int64, text string) error {
	params := url.Values{}
	params.Set("chat_id", chatID)
	params.Set("message_id", strconv.FormatInt(messageID, 10))
	params.Set("text", text)
	_, err := c.call(ctx, "editMessageText", params, nil)
	return err
}
