// OAuth2 redirect and callback handlers for Google and GitHub.
//
// Flow:
//  1. User clicks "Continue with Google/GitHub" → browser navigates to /api/auth/{provider}
//  2. handleOAuthRedirect generates a CSRF state token and redirects to the provider.
//  3. Provider authenticates user and redirects back to /api/auth/{provider}/callback
//  4. handleOAuthCallback validates state, exchanges code for token, fetches the user's
//     email from the provider API, finds-or-creates a user row, creates a one-time
//     exchange token, and redirects the browser to /#exchange=TOKEN.
//  5. The React app reads the fragment, POSTs it to /api/auth/exchange which sets the
//     session cookie and returns the token in the response body for localStorage.
//  6. On any error the browser is redirected to /?auth_error=<reason> so the login page
//     can show a message instead of a blank error screen.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

// authError redirects to the login page with a human-readable error message.
// This is always used instead of http.Error so the user never sees a blank page.
func authError(w http.ResponseWriter, r *http.Request, reason string) {
	log.Printf("OAuth error: %s", reason)
	http.Redirect(w, r, "/?auth_error="+url.QueryEscape(reason), http.StatusFound)
}

// handleOAuthRedirect returns a handler that starts the OAuth flow for the given provider.
func (s *Server) handleOAuthRedirect(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := s.oauthConfig(r, provider)
		state := s.newOAuthState()
		log.Printf("OAuth redirect (%s): redirectURI=%s", provider, cfg.RedirectURL)
		http.Redirect(w, r, cfg.AuthCodeURL(state), http.StatusFound)
	}
}

// handleOAuthCallback returns a handler that completes the OAuth flow for the given provider.
func (s *Server) handleOAuthCallback(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Google/GitHub sends error= when the user denied access
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			authError(w, r, errParam)
			return
		}

		// Validate CSRF state
		if !s.validOAuthState(r.URL.Query().Get("state")) {
			authError(w, r, "invalid or expired state — please try again")
			return
		}

		// Exchange authorization code for access token
		cfg := s.oauthConfig(r, provider)
		log.Printf("OAuth callback (%s): state valid, exchanging code for token (redirectURI=%s)", provider, cfg.RedirectURL)
		token, err := cfg.Exchange(context.Background(), r.URL.Query().Get("code"))
		if err != nil {
			log.Printf("OAuth token exchange failed (%s): %v", provider, err)
			authError(w, r, "authentication failed — please try again")
			return
		}

		// Fetch the user's email from the provider API
		email, providerID, err := fetchOAuthUser(provider, token.AccessToken)
		if err != nil || email == "" {
			log.Printf("OAuth user fetch failed (%s): email=%q err=%v", provider, email, err)
			authError(w, r, "could not retrieve your email from "+provider)
			return
		}
		log.Printf("OAuth (%s): got email=%s providerID=%s", provider, email, providerID)

		// Find existing user linked to this provider identity
		user, err := s.users.FindUserByProvider(provider, providerID)
		if err != nil {
			log.Printf("OAuth (%s): FindUserByProvider error: %v", provider, err)
			authError(w, r, "database error")
			return
		}

		if user == nil {
			// Check if a local account exists with the same email
			user, err = s.users.FindUserByEmail(email)
			if err != nil {
				log.Printf("OAuth (%s): FindUserByEmail error: %v", provider, err)
				authError(w, r, "database error")
				return
			}
			if user == nil {
				// First OAuth login — create account automatically
				user, err = s.users.CreateUser(email, "")
				if err != nil {
					log.Printf("OAuth (%s): CreateUser error: %v", provider, err)
					authError(w, r, "could not create account")
					return
				}
				log.Printf("OAuth (%s): created new user id=%d email=%s", provider, user.ID, email)
			}
			// Link this provider so future logins skip the email lookup
			_ = s.users.LinkProvider(user.ID, provider, providerID)
		}
		log.Printf("OAuth (%s): user id=%d authenticated", provider, user.ID)

		exchangeTok := s.newExchangeToken(user.ID)
		// Use the URL fragment (#exchange=TOKEN) instead of a query parameter.
		// Cloudflare Tunnel strips query parameters from redirect Location headers,
		// but fragments are browser-only — they are never sent to or modified by any proxy.
		http.Redirect(w, r, "/#exchange="+exchangeTok, http.StatusFound)
	}
}

// fetchOAuthUser calls the provider's user-info API and returns (email, providerUserID).
func fetchOAuthUser(provider, accessToken string) (email, id string, err error) {
	switch provider {
	case "google":
		return fetchGoogleUser(accessToken)
	case "github":
		return fetchGitHubUser(accessToken)
	}
	return "", "", nil
}

func fetchGoogleUser(accessToken string) (email, id string, err error) {
	req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", "", err
	}
	return info.Email, info.Sub, nil
}

func fetchGitHubUser(accessToken string) (email, id string, err error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/user", nil)
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var user struct {
		ID    int    `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &user); err != nil {
		return "", "", err
	}
	idStr := fmt.Sprintf("%d", user.ID)
	if user.Email != "" {
		return user.Email, idStr, nil
	}
	// Email may be private — fetch the primary address from /user/emails
	req2, _ := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	req2.Header.Set("Authorization", "token "+accessToken)
	req2.Header.Set("Accept", "application/vnd.github.v3+json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return "", idStr, err
	}
	defer resp2.Body.Close()
	body2, _ := io.ReadAll(resp2.Body)
	var emails []struct {
		Email   string `json:"email"`
		Primary bool   `json:"primary"`
	}
	if err := json.Unmarshal(body2, &emails); err != nil {
		return "", idStr, err
	}
	for _, e := range emails {
		if e.Primary {
			return e.Email, idStr, nil
		}
	}
	if len(emails) > 0 {
		return emails[0].Email, idStr, nil
	}
	return "", idStr, nil
}
