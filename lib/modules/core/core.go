package core

import (
	"github.com/ryankurte/authplz/lib/api"
	"github.com/ryankurte/authplz/lib/events"
)

// Controller core module instance storage
// The core module implements basic login/logout methods and allows binding of modules
// To interrupt/assist/log the execution of each
type Controller struct {
	// Token controller for parsing of tokens
	tokenControl TokenValidator

	// User controller interface for basic user logins
	userControl LoginProvider

	// Token handler implementations
	// This allows token handlers to be bound on a per-module basis using the actions
	// defined in api.TokenAction. Note that there must not be overlaps in bindings
	// TODO: this should probably be implemented as a bind function to panic if overlap is attempted
	tokenHandlers map[api.TokenAction]TokenHandler

	// 2nd Factor Authentication implementations
	secondFactorHandlers map[string]SecondFactorProvider

	// Event handler implementations
	eventHandlers map[string]EventHandler

	// Login handler implementations
	preLogin         map[string]PreLoginHook
	postLoginSuccess map[string]PostLoginSuccessHook
	postLoginFailure map[string]PostLoginFailureHook
}

// NewController Create a new core module instance
func NewController(tokenValidator TokenValidator, loginProvider LoginProvider, emitter events.EventEmitter) *Controller {
	return &Controller{
		tokenControl:         tokenValidator,
		userControl:          loginProvider,
		tokenHandlers:        make(map[api.TokenAction]TokenHandler),
		secondFactorHandlers: make(map[string]SecondFactorProvider),

		preLogin:         make(map[string]PreLoginHook),
		postLoginSuccess: make(map[string]PostLoginSuccessHook),
		postLoginFailure: make(map[string]PostLoginFailureHook),
		eventHandlers:    make(map[string]EventHandler),
	}
}
