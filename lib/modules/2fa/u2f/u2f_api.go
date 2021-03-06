/*
 * U2F / Fido Module API implementation
 * This provides U2F endpoints for device registration, authentication and management
 *
 * AuthPlz Project (https://github.com/ryankurte/AuthPlz)
 * Copyright 2017 Ryan Kurte
 */

// TODO: move all database operations and things into the controller.

package u2f

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)
import (
	"github.com/gocraft/web"
	"github.com/ryankurte/go-u2f"

	"github.com/ryankurte/authplz/lib/api"
	"github.com/ryankurte/authplz/lib/appcontext"
)

const (
	u2fRegisterSessionKey   string = "u2f-register-session"
	u2fSignSessionKey       string = "u2f-sign-session"
	u2fRegisterChallengeKey string = "u2f-register-challenge"
	u2fRegisterNameKey      string = "u2f-register-name"
	u2fSignChallengeKey     string = "u2f-sign-challenge"
	u2fSignUserIDKey        string = "u2f-sign-userid"
	u2fSignActionKey        string = "u2f-sign-action"
)

// apiCtx context storage for router instance
type apiCtx struct {
	// Base context for shared components
	*appcontext.AuthPlzCtx

	// U2F controller module
	um *Controller
}

// Initialise serialisation of u2f challenge objects
func init() {
	gob.Register(&u2f.Challenge{})
}

// BindU2FContext Helper middleware to bind module to API context
func BindU2FContext(u2fModule *Controller) func(ctx *apiCtx, rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
	return func(ctx *apiCtx, rw web.ResponseWriter, req *web.Request, next web.NextMiddlewareFunc) {
		ctx.um = u2fModule
		next(rw, req)
	}
}

// BindAPI Binds the API for the u2f module to the provided router
func (u2fModule *Controller) BindAPI(router *web.Router) {
	// Create router for user modules
	u2frouter := router.Subrouter(apiCtx{}, "/api/u2f")

	// Attach module context
	u2frouter.Middleware(BindU2FContext(u2fModule))

	// Bind endpoints
	u2frouter.Get("/enrol", (*apiCtx).U2FEnrolGet)
	u2frouter.Post("/enrol", (*apiCtx).U2FEnrolPost)
	u2frouter.Get("/authenticate", (*apiCtx).U2FAuthenticateGet)
	u2frouter.Post("/authenticate", (*apiCtx).U2FAuthenticatePost)
	u2frouter.Get("/tokens", (*apiCtx).U2FTokensGet)
}

// U2FEnrolGet First stage token enrolment (get) handler
// This creates and caches a challenge for a device to be registered
func (c *apiCtx) U2FEnrolGet(rw web.ResponseWriter, req *web.Request) {
	// Check if user is logged in
	if c.GetUserID() == "" {
		c.WriteApiResultWithCode(rw, http.StatusUnauthorized, api.ResultError, c.GetAPILocale().Unauthorized)
		return
	}

	tokenName := req.URL.Query().Get("name")
	if tokenName == "" {
		rw.WriteHeader(http.StatusBadRequest)
		c.WriteApiResultWithCode(rw, http.StatusBadRequest, api.ResultError, "Missing query name parameter")
		return
	}

	// Build U2F challenge
	challenge, err := c.um.GetChallenge(c.GetUserID())
	if err != nil {
		c.WriteApiResultWithCode(rw, http.StatusInternalServerError, api.ResultError, c.GetAPILocale().InternalError)
		return
	}
	u2fReq := challenge.RegisterRequest()

	// Save to session
	c.GetSession().Values[u2fRegisterChallengeKey] = challenge
	c.GetSession().Values[u2fRegisterNameKey] = tokenName
	c.GetSession().Save(req.Request, rw)

	log.Println("U2FEnrolGet: Fetched enrolment challenge")

	// Return challenge to user
	c.WriteJson(rw, *u2fReq)
}

// U2FEnrolPost Second stage token enrolment (post) handler
// This checks the cached challenge and completes device enrolment
func (c *apiCtx) U2FEnrolPost(rw web.ResponseWriter, req *web.Request) {

	// Check if user is logged in
	if c.GetUserID() == "" {
		c.WriteApiResult(rw, api.ResultError, c.GetAPILocale().Unauthorized)
		return
	}

	// Fetch request from session vars
	// TODO: move this to a separate session flash
	if c.GetSession().Values[u2fRegisterChallengeKey] == nil {
		c.WriteApiResult(rw, api.ResultError, "No challenge found")
		fmt.Println("No challenge found in session flash")
		return
	}
	challenge := c.GetSession().Values[u2fRegisterChallengeKey].(*u2f.Challenge)
	c.GetSession().Values[u2fRegisterChallengeKey] = ""

	keyName := c.GetSession().Values[u2fRegisterNameKey].(string)
	c.GetSession().Values[u2fRegisterNameKey] = ""

	// Parse JSON response body
	var registerResp u2f.RegisterResponse
	jsonErr := json.NewDecoder(req.Body).Decode(&registerResp)
	if jsonErr != nil {
		c.WriteApiResult(rw, api.ResultError, "Invalid U2F registration response")
		return
	}

	// Validate registration
	ok, err := c.um.ValidateRegistration(c.GetUserID(), keyName, challenge, &registerResp)
	if err != nil {
		c.WriteApiResult(rw, api.ResultError, c.GetAPILocale().InternalError)
		return
	}
	if !ok {
		log.Printf("U2F enrolment failed for user %s\n", c.GetUserID())
		c.WriteApiResult(rw, api.ResultError, c.GetAPILocale().U2FRegistrationFailed)
		return
	}

	log.Printf("Enrolled U2F token for account %s\n", c.GetUserID())
	c.WriteApiResult(rw, api.ResultOk, c.GetAPILocale().U2FRegistrationComplete)
	return
}

// U2FAuthenticateGet Fetches an authentication challenge
// This grabs a pending 2fa userid from the global context
// Not sure how to:
// a) do this better / without global context
// b) allow this to be used for authentication and for "sudo" like behaviour.
func (c *apiCtx) U2FAuthenticateGet(rw web.ResponseWriter, req *web.Request) {
	u2fSession, _ := c.Global.SessionStore.Get(req.Request, u2fSignSessionKey)

	// Fetch challenge user ID
	userid, action := c.Get2FARequest(rw, req)

	if userid == "" {
		log.Printf("u2f.U2FAuthenticateGet No pending 2fa requests found")
		c.WriteApiResultWithCode(rw, http.StatusBadRequest, api.ResultError, "No pending 2fa authorizations found")
		return
	}

	log.Printf("u2f.U2FAuthenticateGet Authentication request for user %s (action %s)", userid, action)

	// Generate challenge
	challenge, err := c.um.GetChallenge(userid)
	if err != nil {
		c.WriteApiResult(rw, api.ResultError, c.GetAPILocale().Unauthorized)
		return
	}
	u2fSignReq := challenge.SignRequest()

	// Save to session vars
	u2fSession.Values[u2fSignChallengeKey] = challenge
	u2fSession.Values[u2fSignUserIDKey] = userid
	u2fSession.Values[u2fSignActionKey] = action
	u2fSession.Save(req.Request, rw)

	// Write challenge to user
	c.WriteJson(rw, *u2fSignReq)
}

// U2FAuthenticatePost Post authentication response to complete authentication
func (c *apiCtx) U2FAuthenticatePost(rw web.ResponseWriter, req *web.Request) {

	u2fSession, _ := c.Global.SessionStore.Get(req.Request, u2fSignSessionKey)

	// Fetch request from session vars
	// TODO: move this to a separate session flash
	if u2fSession.Values[u2fSignChallengeKey] == nil {
		c.WriteApiResult(rw, api.ResultError, "No challenge found")
		fmt.Println("No challenge found in session flash")
		return
	}
	challenge := u2fSession.Values[u2fSignChallengeKey].(*u2f.Challenge)
	u2fSession.Values[u2fSignChallengeKey] = ""

	if u2fSession.Values[u2fSignUserIDKey] == nil {
		c.WriteApiResult(rw, api.ResultError, "No userid found")
		fmt.Println("No userid found in session flash")
		return
	}
	userid := u2fSession.Values[u2fSignUserIDKey].(string)
	u2fSession.Values[u2fSignUserIDKey] = ""

	action := u2fSession.Values[u2fSignActionKey].(string)
	u2fSession.Values[u2fSignActionKey] = ""

	// Clear session vars
	u2fSession.Save(req.Request, rw)

	log.Printf("U2F Authenticate post for user %s (action %s)", userid, action)

	// Parse JSON response body
	var u2fSignResp u2f.SignResponse
	err := json.NewDecoder(req.Body).Decode(&u2fSignResp)
	if err != nil {
		log.Printf("U2FAuthenticatePost: error decoding sign response (%s)", err)
		c.WriteApiResult(rw, api.ResultError, "Invalid U2F registration response")
		return
	}

	// Validate signature
	ok, err := c.um.ValidateSignature(userid, challenge, &u2fSignResp)
	if err != nil {
		c.WriteApiResult(rw, api.ResultError, c.GetAPILocale().InternalError)
		return
	}
	if !ok {
		log.Printf("U2FAuthenticatePost: authentication failed for user %s\n", userid)
		c.WriteApiResult(rw, api.ResultError, c.GetAPILocale().U2FRegistrationFailed)
		return
	}

	log.Printf("U2FAuthenticatePost: Valid authentication for account %s (action %s)\n", userid, action)
	c.UserAction(userid, action, rw, req)
	c.WriteApiResult(rw, api.ResultOk, c.GetAPILocale().LoginSuccessful)
}

// U2FTokensGet Lists u2f tokens for the logged in user user
func (c *apiCtx) U2FTokensGet(rw web.ResponseWriter, req *web.Request) {

	// Check if user is logged in
	if c.GetUserID() == "" {
		c.WriteApiResult(rw, api.ResultError, c.GetAPILocale().Unauthorized)
		return
	}

	// Fetch tokens
	tokens, err := c.um.ListTokens(c.GetUserID())
	if err != nil {
		log.Printf("Error fetching U2F tokens %s", err)
		c.WriteApiResult(rw, api.ResultError, c.GetAPILocale().InternalError)
		return
	}

	// Write tokens out
	c.WriteJson(rw, tokens)
}
