// Copyright 2022 Paul Greenberg greenpau@outlook.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"context"
	"encoding/json"
	"go.uber.org/zap"
	"net/http"
	"sync"
	"time"
)

// Endpoint handles git management requests.
type Endpoint struct {
	mu             sync.Mutex
	Name           string `json:"-"`
	RepositoryName string
	logger         *zap.Logger
	startedAt      time.Time
}

// SetLogger add logger to Endpoint.
func (m *Endpoint) SetLogger(logger *zap.Logger) {
	m.logger = logger
}

// Provision configures the instance of Endpoint.
func (m *Endpoint) Provision() error {
	m.startedAt = time.Now().UTC()
	m.Name = "git-" + m.RepositoryName

	m.logger.Info(
		"provisioned plugin instance",
		zap.String("instance_name", m.Name),
		zap.Time("started_at", m.startedAt),
	)
	return nil
}

// Validate implements caddy.Validator.
func (m *Endpoint) Validate() error {
	m.logger.Info(
		"validated plugin instance",
		zap.String("instance_name", m.Name),
	)
	return nil
}

// ServeHTTP serves git management requests.
func (m *Endpoint) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	m.logger.Debug(
		"received update git repo request",
		zap.String("repo_name", m.RepositoryName),
	)

	resp := make(map[string]interface{})
	repo, exists := manager.repos[m.RepositoryName]
	if !exists {
		resp["status_code"] = http.StatusInternalServerError
		m.logger.Warn("repo not found", zap.String("repo_name", m.RepositoryName))
		return m.respondHTTP(ctx, w, r, resp)
	}

	if len(repo.Config.Webhooks) > 0 {
		// Inspect HTTP headers for webhooks.
		var authorized bool
		for _, webhook := range repo.Config.Webhooks {
			hdr := r.Header.Get(webhook.Header)
			if hdr == "" {
				continue
			}
			if hdr != webhook.Secret {
				resp["status_code"] = http.StatusUnauthorized
				m.logger.Warn(
					"webhook authentication failed",
					zap.String("repo_name", repo.Config.Name),
					zap.String("webhook_header", webhook.Header),
					zap.String("error", "auth header value mismatch"),
				)
				return m.respondHTTP(ctx, w, r, resp)
			}
			authorized = true
		}

		if !authorized {
			resp["status_code"] = http.StatusUnauthorized
			m.logger.Warn(
				"webhook authentication failed",
				zap.String("repo_name", repo.Config.Name),
				zap.String("error", "auth header not found"),
			)
			return m.respondHTTP(ctx, w, r, resp)
		}
	}

	if err := repo.update(); err != nil {
		m.logger.Warn("failed updating repo", zap.String("repo_name", repo.Config.Name), zap.Error(err))
		resp["status_code"] = http.StatusInternalServerError
		return m.respondHTTP(ctx, w, r, resp)
	}

	resp["status_code"] = http.StatusOK
	return m.respondHTTP(ctx, w, r, resp)
}

func (m *Endpoint) respondHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request, data map[string]interface{}) error {
	b, _ := json.Marshal(data)
	if code, exists := data["status_code"]; exists {
		w.WriteHeader(code.(int))
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Write(b)
	return nil
}
