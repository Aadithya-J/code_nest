//go:build manual
// +build manual

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type authResp struct {
	Token string `json:"token"`
}

type projectResp struct {
	ProjectID string `json:"projectId"`
}

type startResp struct {
	AtlasID string `json:"atlasId"`
	OK      bool   `json:"ok"`
	Status  string `json:"status"`
}

func main() {
	base := flag.String("base", "http://localhost:3000", "gateway base URL")
	repoURL := flag.String("repo", "https://github.com/Aadithya-J/Snipbox", "repository URL")
	atlasID := flag.String("atlas", "", "atlas id to use (optional; defaults to ws-{projectId})")
	email := flag.String("email", fmt.Sprintf("e2e-%d@example.com", time.Now().Unix()), "email to signup/login")
	password := flag.String("password", "yourpass", "password to signup/login")
	tokenFlag := flag.String("token", "", "existing JWT token (skip signup/login)")
	skipFlag := flag.Bool("skip", false, "skip signup, just login")
	projectName := flag.String("name", fmt.Sprintf("E2E Project %d", time.Now().Unix()), "project name")
	installIDFlag := flag.String("install-id", "", "GitHub installation id (optional; otherwise prompted)")
	flag.Parse()

	client := &http.Client{Timeout: 20 * time.Second}

	// 1) health
	checkHealth(client, *base)

	// 2) signup/login
	var token string
	if *tokenFlag != "" {
		token = *tokenFlag
		fmt.Printf("Using provided token\n")
	} else if *skipFlag {
		token = login(client, *base, *email, *password)
	} else {
		signup(client, *base, *email, *password)
		token = login(client, *base, *email, *password)
	}
	fmt.Printf("Using token (sub for this user): %s...\n", preview(token))

	// 3) get GitHub URL with state and guide user through install
	githubURL, state := getGitHubURL(client, *base, token)
	fmt.Println("Open this GitHub App install URL in a browser and complete the install:")
	fmt.Println("  ", githubURL)
	fmt.Println("After installing, paste the installation_id (or the redirected URL) below and press Enter.")

	installationID := *installIDFlag
	if installationID == "" {
		fmt.Print("installation_id or URL: ")
		raw, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		installationID = strings.TrimSpace(raw)
	}
	installationID = extractInstallationID(installationID)
	if installationID == "" {
		fmt.Println("Could not determine installation_id; aborting.")
		os.Exit(1)
	}

	// Hit the callback ourselves to record the installation for this user
	if err := triggerGitHubCallback(client, *base, state, installationID); err != nil {
		fmt.Printf("github callback error: %v\n", err)
		os.Exit(1)
	}

	// 4) create project
	projectID := createProject(client, *base, token, *projectName, *repoURL)

	// 5) start workspace
	startWorkspace(client, *base, token, projectID, *atlasID)

	fmt.Println("Done. Now check agent:")
	fmt.Printf("  Files:  http://ws-%s.127.0.0.1.nip.io:9000/files\n", projectID)
	fmt.Printf("  Health: http://ws-%s.127.0.0.1.nip.io:9000/health\n", projectID)
}

func checkHealth(client *http.Client, base string) {
	resp, err := client.Get(base + "/api/health")
	if err != nil {
		fmt.Printf("health check failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("health check returned %d\n", resp.StatusCode)
		os.Exit(1)
	}
	fmt.Println("Health OK")
}

func signup(client *http.Client, base, email, password string) string {
	body := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)
	resp, err := client.Post(base+"/api/auth/signup", "application/json", bytes.NewBufferString(body))
	if err != nil {
		fmt.Printf("signup error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("signup status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	var ar authResp
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil || ar.Token == "" {
		fmt.Printf("signup decode error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Signup OK for %s\n", email)
	return ar.Token
}

func login(client *http.Client, base, email, password string) string {
	body := fmt.Sprintf(`{"email":"%s","password":"%s"}`, email, password)
	resp, err := client.Post(base+"/api/auth/login", "application/json", bytes.NewBufferString(body))
	if err != nil {
		fmt.Printf("login error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("login status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	var ar authResp
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil || ar.Token == "" {
		fmt.Printf("login decode error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Login OK for %s\n", email)
	return ar.Token
}

func getGitHubURL(client *http.Client, base, token string) (string, string) {
	req, _ := http.NewRequest("GET", base+"/api/auth/github/url", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("github url error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("github url status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	var m map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil || m["url"] == "" {
		fmt.Printf("github url decode error: %v\n", err)
		os.Exit(1)
	}
	state := parseState(m["url"])
	if state == "" {
		fmt.Println("GitHub install URL missing state; ensure Authorization header was accepted.")
		os.Exit(1)
	}
	return m["url"], state
}

func createProject(client *http.Client, base, token, name, repo string) string {
	body := fmt.Sprintf(`{"name":"%s","repoUrl":"%s"}`, name, repo)
	req, _ := http.NewRequest("POST", base+"/api/projects", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("create project error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("create project status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	var pr projectResp
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil || pr.ProjectID == "" {
		fmt.Printf("create project decode error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Project created:", pr.ProjectID)
	return pr.ProjectID
}

func startWorkspace(client *http.Client, base, token, projectID, atlasID string) {
	body := `{}`
	if atlasID != "" {
		body = fmt.Sprintf(`{"atlas_id":"%s"}`, atlasID)
	}
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/projects/%s/start", base, projectID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("start workspace error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, resp.Body)
		fmt.Printf("start workspace status %d body: %s\n", resp.StatusCode, buf.String())
		os.Exit(1)
	}
	var sr startResp
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		fmt.Printf("start workspace decode error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Workspace start: atlasId=%s status=%s ok=%v\n", sr.AtlasID, sr.Status, sr.OK)
}

func preview(token string) string {
	if len(token) <= 16 {
		return token
	}
	return token[:8] + "..." + token[len(token)-8:]
}

func parseState(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("state")
}

func extractInstallationID(input string) string {
	// If the input is already numeric, return it
	if _, err := strconv.ParseInt(input, 10, 64); err == nil {
		return input
	}

	// Try to parse from URL query or path
	if u, err := url.Parse(strings.TrimSpace(input)); err == nil {
		if id := u.Query().Get("installation_id"); id != "" {
			return id
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		// Look for .../installations/<id>
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "installations" && i+1 < len(parts) {
				if _, err := strconv.ParseInt(parts[i+1], 10, 64); err == nil {
					return parts[i+1]
				}
			}
		}
	}
	return ""
}

func triggerGitHubCallback(client *http.Client, base, state, installationID string) error {
	if state == "" || installationID == "" {
		return fmt.Errorf("state or installation_id missing")
	}
	callbackURL := fmt.Sprintf("%s/api/auth/github/callback?state=%s&installation_id=%s", base, url.QueryEscape(state), url.QueryEscape(installationID))
	resp, err := client.Get(callbackURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("callback status %d body: %s", resp.StatusCode, string(body))
	}
	return nil
}
