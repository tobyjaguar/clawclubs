package server_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tobyjaguar/clawclubs/internal/auth"
	"github.com/tobyjaguar/clawclubs/internal/models"
	"github.com/tobyjaguar/clawclubs/internal/server"
	"github.com/tobyjaguar/clawclubs/internal/store"
)

const testAdminKey = "test-admin-key"

func setup(t *testing.T) *httptest.Server {
	t.Helper()
	auth.ResetNonces()
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	srv := server.New(st, testAdminKey)
	return httptest.NewServer(srv)
}

func adminPost(t *testing.T, ts *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+testAdminKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func signedRequest(t *testing.T, method, url string, privKey ed25519.PrivateKey, body []byte) *http.Request {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	auth.SignRequest(req, privKey, body)
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestFullFlow(t *testing.T) {
	ts := setup(t)
	defer ts.Close()

	// 1. Landing page
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("landing: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("landing status: %d", resp.StatusCode)
	}

	// 2. Create a club
	resp = adminPost(t, ts, "/admin/clubs", map[string]string{
		"name":        "Test Club",
		"description": "A test club",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create club status: %d", resp.StatusCode)
	}
	var club models.Club
	json.NewDecoder(resp.Body).Decode(&club)
	resp.Body.Close()
	if club.ID == "" || club.Name != "Test Club" {
		t.Fatalf("unexpected club: %+v", club)
	}

	// 3. Create an invite
	resp = adminPost(t, ts, "/admin/invites", map[string]any{
		"club_id":  club.ID,
		"max_uses": 5,
	})
	if resp.StatusCode != 201 {
		t.Fatalf("create invite status: %d", resp.StatusCode)
	}
	var invite models.Invite
	json.NewDecoder(resp.Body).Decode(&invite)
	resp.Body.Close()
	if invite.Code == "" {
		t.Fatalf("invite code empty")
	}

	// 4. Enroll an agent
	pub, priv, _ := ed25519.GenerateKey(nil)
	agentID := hex.EncodeToString(pub)

	enrollBody, _ := json.Marshal(map[string]string{
		"invite_code":  invite.Code,
		"agent_pubkey": agentID,
		"agent_name":   "TestAgent",
	})
	enrollReq, _ := http.NewRequest("POST", ts.URL+"/clubs/"+club.ID+"/enroll", bytes.NewReader(enrollBody))
	enrollReq.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(enrollReq)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("enroll status: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Post a message (agent-authenticated)
	msgBody := []byte(`{"content":"Hello from TestAgent!"}`)
	req := signedRequest(t, "POST", ts.URL+"/clubs/"+club.ID+"/messages", priv, msgBody)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post message: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("post message status: %d", resp.StatusCode)
	}
	var msg models.Message
	json.NewDecoder(resp.Body).Decode(&msg)
	resp.Body.Close()
	if msg.Content != "Hello from TestAgent!" {
		t.Fatalf("unexpected message: %+v", msg)
	}

	// 6. Get messages
	req = signedRequest(t, "GET", ts.URL+"/clubs/"+club.ID+"/messages", priv, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get messages: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("get messages status: %d", resp.StatusCode)
	}
	var msgs []models.Message
	json.NewDecoder(resp.Body).Decode(&msgs)
	resp.Body.Close()
	if len(msgs) != 1 || msgs[0].Content != "Hello from TestAgent!" {
		t.Fatalf("unexpected messages: %+v", msgs)
	}

	// 7. List clubs for agent
	req = signedRequest(t, "GET", ts.URL+"/clubs", priv, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list clubs: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("list clubs status: %d", resp.StatusCode)
	}
	var clubs []models.Club
	json.NewDecoder(resp.Body).Decode(&clubs)
	resp.Body.Close()
	if len(clubs) != 1 || clubs[0].Name != "Test Club" {
		t.Fatalf("unexpected clubs: %+v", clubs)
	}

	// 8. Get club details
	req = signedRequest(t, "GET", ts.URL+"/clubs/"+club.ID, priv, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get club: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("get club status: %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUnauthenticatedAccess(t *testing.T) {
	ts := setup(t)
	defer ts.Close()

	// Trying to list clubs without auth should fail
	resp, err := http.Get(ts.URL + "/clubs")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminAuthRequired(t *testing.T) {
	ts := setup(t)
	defer ts.Close()

	// Trying admin endpoint without key
	body, _ := json.Marshal(map[string]string{"name": "test"})
	req, _ := http.NewRequest("POST", ts.URL+"/admin/clubs", bytes.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestReplayRejection(t *testing.T) {
	ts := setup(t)
	defer ts.Close()

	// Setup: create club, invite, enroll agent
	resp := adminPost(t, ts, "/admin/clubs", map[string]string{
		"name": "Replay Test Club", "description": "test",
	})
	var club models.Club
	json.NewDecoder(resp.Body).Decode(&club)
	resp.Body.Close()

	resp = adminPost(t, ts, "/admin/invites", map[string]any{
		"club_id": club.ID, "max_uses": 5,
	})
	var invite models.Invite
	json.NewDecoder(resp.Body).Decode(&invite)
	resp.Body.Close()

	pub, priv, _ := ed25519.GenerateKey(nil)
	agentID := hex.EncodeToString(pub)
	enrollBody, _ := json.Marshal(map[string]string{
		"invite_code": invite.Code, "agent_pubkey": agentID, "agent_name": "ReplayAgent",
	})
	enrollReq, _ := http.NewRequest("POST", ts.URL+"/clubs/"+club.ID+"/enroll", bytes.NewReader(enrollBody))
	enrollReq.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(enrollReq)

	// First request should succeed
	req := signedRequest(t, "GET", ts.URL+"/clubs", priv, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("first request status: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Replay the exact same request (same nonce) — should get 401
	// We need to rebuild the request body reader since it was consumed
	replayReq, _ := http.NewRequest(req.Method, req.URL.String(), bytes.NewReader(nil))
	replayReq.Header = req.Header.Clone()
	resp, err = http.DefaultClient.Do(replayReq)
	if err != nil {
		t.Fatalf("replay request: %v", err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("replay request: expected 401, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRateLimiting(t *testing.T) {
	ts := setup(t)
	defer ts.Close()

	// Burst enrollment requests from same IP — should hit 429 after burst limit
	var got429 bool
	for i := 0; i < 15; i++ {
		body, _ := json.Marshal(map[string]string{
			"invite_code": "fake", "agent_pubkey": "fake", "agent_name": "test",
		})
		req, _ := http.NewRequest("POST", ts.URL+"/clubs/fakeid/enroll", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if resp.StatusCode == 429 {
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter != "1" {
				t.Fatalf("expected Retry-After: 1, got %s", retryAfter)
			}
			got429 = true
			resp.Body.Close()
			break
		}
		resp.Body.Close()
	}
	if !got429 {
		t.Fatal("expected 429 after burst of enrollment requests")
	}
}
