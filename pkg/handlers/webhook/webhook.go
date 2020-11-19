/*
Copyright 2018 Bitnami

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bitnami-labs/kubewatch/config"
	"github.com/bitnami-labs/kubewatch/pkg/event"
)

var webhookErrMsg = `
%s

You need to set Webhook url
using "--url/-u" or using environment variables:

export KW_WEBHOOK_URL=webhook_url

Command line flags will override environment variables

`

// Webhook handler implements handler.Handler interface,
// Notify event to Webhook channel
type Webhook struct {
	Url                 string
	HMACKey             []byte
	HMACSignatureHeader string
}

// WebhookMessage for messages
type WebhookMessage struct {
	EventMeta EventMeta `json:"eventmeta"`
	Text      string    `json:"text"`
	Time      time.Time `json:"time"`
}

// EventMeta containes the meta data about the event occurred
type EventMeta struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
}

// Init prepares Webhook configuration
func (m *Webhook) Init(c *config.Config) error {
	url := c.Handler.Webhook.Url
	hmacKey := c.Handler.Webhook.HMACKey
	hmacSignatureHeader := c.Handler.Webhook.HMACSignatureHeader

	if url == "" {
		url = os.Getenv("KW_WEBHOOK_URL")
	}

	if hmacKey == "" {
		hmacKey = os.Getenv("KW_WEBHOOK_HMAC_KEY")
	}

	if hmacSignatureHeader == "" {
		hmacSignatureHeader = os.Getenv("KW_WEBHOOK_HMAC_SIGNATURE_HEADER")
		if hmacSignatureHeader == "" {
			hmacSignatureHeader = "X-KubeWatch-Signature"
		}
	}

	m.Url = url
	m.HMACSignatureHeader = hmacSignatureHeader

	if hmacKey != "" {
		hmacKeyDecoded, err := base64.StdEncoding.DecodeString(hmacKey)
		if err != nil {
			return err
		}
		m.HMACKey = hmacKeyDecoded
	}

	return checkMissingWebhookVars(m)
}

// Handle handles an event.
func (m *Webhook) Handle(e event.Event) {
	webhookMessage := prepareWebhookMessage(e, m)

	err := postMessage(m.Url, m.HMACKey, m.HMACSignatureHeader, webhookMessage)
	if err != nil {
		log.Printf("%s\n", err)
		return
	}

	log.Printf("Message successfully sent to %s at %s ", m.Url, time.Now())
}

func checkMissingWebhookVars(s *Webhook) error {
	if s.Url == "" {
		return fmt.Errorf(webhookErrMsg, "Missing Webhook url")
	}

	return nil
}

func prepareWebhookMessage(e event.Event, m *Webhook) *WebhookMessage {
	return &WebhookMessage{
		EventMeta: EventMeta{
			Kind:      e.Kind,
			Name:      e.Name,
			Namespace: e.Namespace,
			Reason:    e.Reason,
		},
		Text: e.Message(),
		Time: time.Now(),
	}
}

func postMessage(url string, hmacKey []byte, hmacSignatureHeader string, webhookMessage *WebhookMessage) error {
	message, err := json.Marshal(webhookMessage)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(message))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	if hmacKey != nil {
		signature := getWebhookMessageSignature(hmacKey, message)
		req.Header.Add(hmacSignatureHeader, signature)
	}

	client := &http.Client{}
	_, err = client.Do(req)
	if err != nil {
		return err
	}

	return nil
}

func getWebhookMessageSignature(hmacKey, data []byte) string {
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write(data)

	return hex.EncodeToString(mac.Sum(nil))
}
