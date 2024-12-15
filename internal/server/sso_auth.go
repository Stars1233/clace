// Copyright (c) ClaceIO, LLC
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"

	"github.com/claceio/clace/internal/types"
	"github.com/go-chi/chi"
	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"

	"github.com/markbates/goth/providers/amazon"
	"github.com/markbates/goth/providers/auth0"
	"github.com/markbates/goth/providers/azuread"
	"github.com/markbates/goth/providers/bitbucket"
	"github.com/markbates/goth/providers/digitalocean"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/gitlab"
	"github.com/markbates/goth/providers/google"
	"github.com/markbates/goth/providers/microsoftonline"
	"github.com/markbates/goth/providers/okta"
	"github.com/markbates/goth/providers/openidConnect"
)

const (
	PROVIDER_NAME_DELIMITER = "_"
	SESSION_COOKIE          = "clace_session"
	AUTH_KEY                = "authenticated"
	USER_ID_KEY             = "user"
	USER_EMAIL_KEY          = "email"
	USER_NICKNAME_KEY       = "nickname"
	PROVIDER_NAME_KEY       = "provider_name"
	REDIRECT_URL            = "redirect"
)

type SSOAuth struct {
	*types.Logger
	config          *types.ServerConfig
	cookieStore     *sessions.CookieStore
	providerConfigs map[string]*types.AuthConfig
}

func NewSSOAuth(logger *types.Logger, config *types.ServerConfig) *SSOAuth {
	return &SSOAuth{
		Logger: logger,
		config: config,
	}
}

func getProviderName(r *http.Request) (string, error) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		return "", fmt.Errorf("provider not specified in url")
	}
	return provider, nil
}

func genCookieName(provider string) string {
	return fmt.Sprintf("%s_%s", provider, SESSION_COOKIE)
}

func generateRandomKey(length int) (string, error) {
	key := make([]byte, length)
	_, err := rand.Read(key)
	if err != nil {
		return "", err
	}
	return string(key), nil
}

func (s *SSOAuth) Setup() error {
	var err error
	sessionKey := s.config.Security.SessionSecret
	if sessionKey == "" {
		sessionKey, err = generateRandomKey(32)
		if err != nil {
			return err
		}
	}

	sessionBlockKey := s.config.Security.SessionBlockKey
	if sessionBlockKey == "" {
		sessionBlockKey, err = generateRandomKey(32)
		if err != nil {
			return err
		}
	}

	s.cookieStore = sessions.NewCookieStore([]byte(sessionKey), []byte(sessionBlockKey))
	s.cookieStore.MaxAge(s.config.Security.SessionMaxAge)
	s.cookieStore.Options.Path = "/"
	s.cookieStore.Options.HttpOnly = true
	s.cookieStore.Options.Secure = s.config.Security.SessionHttpsOnly
	s.cookieStore.Options.SameSite = http.SameSiteLaxMode

	gothic.Store = s.cookieStore // Set the store for gothic
	gothic.GetProviderName = getProviderName
	s.providerConfigs = make(map[string]*types.AuthConfig)

	providers := make([]goth.Provider, 0)
	for providerName, auth := range s.config.Auth {
		auth := auth
		key := auth.Key
		secret := auth.Secret
		scopes := auth.Scopes

		if providerName == "" || key == "" || secret == "" {
			return fmt.Errorf("provider, key, and secret must be set for each auth provider")
		}

		callbackUrl := s.config.Security.CallbackUrl + types.INTERNAL_URL_PREFIX + "/auth/" + providerName + "/callback"

		providerSplit := strings.SplitN(providerName, PROVIDER_NAME_DELIMITER, 2)
		providerType := providerSplit[0]

		var provider goth.Provider
		switch providerType {
		case "github":
			provider = github.New(key, secret, callbackUrl, scopes...)
		case "google": // google supports hosted domain option
			gp := google.New(key, secret, callbackUrl, scopes...)
			if auth.HostedDomain != "" {
				gp.SetHostedDomain(auth.HostedDomain)
			}
			provider = gp
		case "digitalocean":
			provider = digitalocean.New(key, secret, callbackUrl, scopes...)
		case "bitbucket":
			provider = bitbucket.New(key, secret, callbackUrl, scopes...)
		case "amazon":
			provider = amazon.New(key, secret, callbackUrl, scopes...)
		case "azuread": // azuread requires a resources array, setting nil for now
			provider = azuread.New(key, secret, callbackUrl, nil, scopes...)
		case "microsoftonline":
			provider = microsoftonline.New(key, secret, callbackUrl, scopes...)
		case "gitlab":
			provider = gitlab.New(key, secret, callbackUrl, scopes...)
		case "auth0": // auth0 requires a domain
			provider = auth0.New(key, secret, callbackUrl, auth.Domain, scopes...)
		case "okta": // okta requires an org url
			provider = okta.New(key, secret, callbackUrl, auth.OrgUrl, scopes...)
		case "oidc": // openidConnect requires a discovery url
			op, err := openidConnect.New(key, secret, callbackUrl, auth.DiscoveryUrl, scopes...)
			if err != nil {
				return fmt.Errorf("failed to create OIDC provider: %w", err)
			}
			provider = op
		default:
			return fmt.Errorf("unsupported auth provider: %s", providerName)
		}

		provider.SetName(providerName)
		providers = append(providers, provider)
		s.providerConfigs[providerName] = &auth
	}

	if len(providers) != 0 && s.config.Security.CallbackUrl == "" {
		return fmt.Errorf("security.callback_url must be set for enabling SSO auth")
	}

	goth.UseProviders(providers...) // Register the providers with goth
	return nil
}

func (s *SSOAuth) RegisterRoutes(mux *chi.Mux) {
	mux.Get(types.INTERNAL_URL_PREFIX+"/auth/{provider}/callback", func(w http.ResponseWriter, r *http.Request) {
		user, err := gothic.CompleteUserAuth(w, r)
		if err != nil {
			fmt.Fprintln(w, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		providerName := chi.URLParam(r, "provider")
		if err := s.validateResponse(providerName, user); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}

		// Set user as authenticated in session
		cookieName := genCookieName(providerName)
		session, err := s.cookieStore.Get(r, cookieName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		session.Values[AUTH_KEY] = true
		session.Values[USER_ID_KEY] = user.UserID
		session.Values[USER_EMAIL_KEY] = user.Email
		session.Values[USER_NICKNAME_KEY] = user.NickName
		session.Values[PROVIDER_NAME_KEY] = providerName
		session.Save(r, w)

		// Redirect to the original page, or default to the home page if not specified
		redirectTo, ok := session.Values[REDIRECT_URL].(string)
		if !ok || redirectTo == "" {
			redirectTo = "/"
		}

		http.Redirect(w, r, redirectTo, http.StatusTemporaryRedirect)
	})

	mux.Post(types.INTERNAL_URL_PREFIX+"/logout/{provider}", func(w http.ResponseWriter, r *http.Request) {
		if err := gothic.Logout(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Set user as not authenticated in session
		providerName := chi.URLParam(r, "provider")
		cookieName := genCookieName(providerName)
		session, err := s.cookieStore.Get(r, cookieName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Set user as unauthenticated in session
		session.Values[AUTH_KEY] = false
		session.Save(r, w)

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	})

	mux.Get(types.INTERNAL_URL_PREFIX+"/auth/{provider}", func(w http.ResponseWriter, r *http.Request) {
		providerName := chi.URLParam(r, "provider")
		// try to get the user without re-authenticating
		if _, err := gothic.CompleteUserAuth(w, r); err == nil {
			userId, err := s.CheckAuth(w, r, providerName, false)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if userId != "" {
				cookieName := genCookieName(providerName)
				session, err := s.cookieStore.Get(r, cookieName)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				// Redirect to the original page, or default to the home page if not specified
				redirectTo, ok := session.Values[REDIRECT_URL].(string)
				if !ok || redirectTo == "" {
					redirectTo = "/"
				}

				http.Redirect(w, r, redirectTo, http.StatusTemporaryRedirect)
				return
			}
		}

		// Start login process
		gothic.BeginAuthHandler(w, r)
	})
}

func (s *SSOAuth) validateResponse(providerName string, user goth.User) error {
	providerConfig := s.providerConfigs[providerName]
	if providerConfig == nil {
		return fmt.Errorf("provider %s not configured", providerName)
	}

	providerType := strings.SplitN(providerName, PROVIDER_NAME_DELIMITER, 2)[0]
	switch providerType {
	case "google":
		if providerConfig.HostedDomain != "" && user.RawData["hd"] != providerConfig.HostedDomain {
			return fmt.Errorf("user does not belong to the required hosted domain. Found %s, expected %s",
				user.RawData["hd"], providerConfig.HostedDomain)
		}
	}

	return nil
}

func (s *SSOAuth) ValidateProviderName(provider string) bool {
	return s.providerConfigs[provider] != nil
}

func (s *SSOAuth) ValidateAuthType(authType string) bool {
	switch authType {
	case string(types.AppAuthnDefault), string(types.AppAuthnSystem), string(types.AppAuthnNone):
		return true
	default:
		if authType == "cert" || strings.HasPrefix(authType, "cert_") {
			_, ok := s.config.ClientAuth[authType]
			return ok
		}
		return s.ValidateProviderName(authType)
	}
}

func (s *SSOAuth) CheckAuth(w http.ResponseWriter, r *http.Request, appProvider string, updateRedirect bool) (string, error) {
	cookieName := genCookieName(appProvider)
	session, err := s.cookieStore.Get(r, cookieName)
	if err != nil {
		s.Warn().Err(err).Msg("failed to get session")
		return "", err
	}
	if auth, ok := session.Values[AUTH_KEY].(bool); !ok || !auth {
		// Store the target URL before redirecting to login
		if updateRedirect {
			session.Values[REDIRECT_URL] = r.RequestURI
			session.Save(r, w)
		}
		s.Warn().Err(err).Msg("no auth, redirecting to login")
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Redirect", types.INTERNAL_URL_PREFIX+"/auth/"+appProvider)
		} else {
			http.Redirect(w, r, types.INTERNAL_URL_PREFIX+"/auth/"+appProvider, http.StatusTemporaryRedirect)
		}
		return "", nil
	}

	// Check if provider name matches the one in the session
	if providerName, ok := session.Values[PROVIDER_NAME_KEY].(string); !ok || providerName != appProvider {
		if updateRedirect {
			session.Values[REDIRECT_URL] = r.RequestURI
			session.Save(r, w)
		}
		s.Warn().Err(err).Msg("provider mismatch, redirecting to login")
		http.Redirect(w, r, types.INTERNAL_URL_PREFIX+"/auth/"+appProvider, http.StatusTemporaryRedirect)
		return "", nil
	}

	userId, ok := session.Values[USER_EMAIL_KEY].(string)
	if !ok || userId == "" {
		userId, ok = session.Values[USER_NICKNAME_KEY].(string)
		if !ok || userId == "" {
			userId, ok = session.Values[USER_ID_KEY].(string)
			if !ok || userId == "" {
				s.Warn().Msg("no user id in session")
				return "", fmt.Errorf("no user id in session")
			}
		}
	}

	// Clear the redirect target after successful authentication
	delete(session.Values, REDIRECT_URL)
	session.Save(r, w)

	return appProvider + ":" + userId, nil
}
