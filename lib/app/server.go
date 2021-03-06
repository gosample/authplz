package app

import (
	"log"
	"net/http"
	"os"
	"path"

	"github.com/gocraft/web"
	"github.com/gorilla/context"
	"github.com/gorilla/sessions"

	"github.com/ryankurte/authplz/lib/api"
	"github.com/ryankurte/authplz/lib/appcontext"
	"github.com/ryankurte/authplz/lib/config"

	"github.com/ryankurte/authplz/lib/controllers/datastore"
	"github.com/ryankurte/authplz/lib/controllers/mailer"
	"github.com/ryankurte/authplz/lib/controllers/token"

	"github.com/ryankurte/authplz/lib/modules/2fa/backup"
	"github.com/ryankurte/authplz/lib/modules/2fa/totp"
	"github.com/ryankurte/authplz/lib/modules/2fa/u2f"

	"github.com/ryankurte/authplz/lib/modules/audit"
	"github.com/ryankurte/authplz/lib/modules/core"
	"github.com/ryankurte/authplz/lib/modules/oauth"
	"github.com/ryankurte/authplz/lib/modules/user"

	"github.com/ryankurte/go-async"
)

// AuthPlzServer Base AuthPlz server object
type AuthPlzServer struct {
	address        string
	port           string
	config         config.AuthPlzConfig
	ds             *datastore.DataStore
	ctx            appcontext.AuthPlzGlobalCtx
	router         *web.Router
	tokenControl   *token.TokenController
	serviceManager *async.ServiceManager
}

const bufferSize uint = 64

// NewServer Create an AuthPlz server instance
func NewServer(config config.AuthPlzConfig) *AuthPlzServer {
	server := AuthPlzServer{}

	server.config = config

	// Attempt database connection
	dataStore, err := datastore.NewDataStore(config.Database)
	if err != nil {
		log.Panic("Error opening database")
	}
	server.ds = dataStore

	// Create session store
	sessionStore := sessions.NewCookieStore([]byte(config.CookieSecret))
	//sessionStore.Options.Secure = true
	//sessionStore.Options.HttpOnly = true

	// Create token controller
	tokenControl := token.NewTokenController(server.config.Address, string(config.TokenSecret), dataStore)
	server.tokenControl = tokenControl

	// TODO: Create CSRF middleware

	// Create modules

	// Create service manager
	server.serviceManager = async.NewServiceManager(bufferSize)

	// User management module
	userModule := user.NewController(dataStore, server.serviceManager)

	// Core module
	coreModule := core.NewController(tokenControl, userModule, server.serviceManager)

	coreModule.BindModule("user", userModule)
	coreModule.BindActionHandler(api.TokenActionActivate, userModule)
	coreModule.BindActionHandler(api.TokenActionUnlock, userModule)

	// 2fa modules
	u2fModule := u2f.NewController(config.ExternalAddress, dataStore, server.serviceManager)
	coreModule.BindSecondFactor("u2f", u2fModule)

	totpModule := totp.NewController(config.Name, dataStore, server.serviceManager)
	coreModule.BindSecondFactor("totp", totpModule)

	backupModule := backup.NewController(config.Name, dataStore, server.serviceManager)
	coreModule.BindSecondFactor("backup", backupModule)

	// Audit module (async components)
	auditModule := audit.NewController(dataStore)
	auditSvc := async.NewAsyncService(auditModule, bufferSize)
	server.serviceManager.BindService(&auditSvc)

	// Mailer module
	mailController, err := mailer.NewMailController(config.Name, config.ExternalAddress, config.Mailer.Driver, config.Mailer.Options, dataStore, tokenControl, config.TemplateDir)
	if err != nil {
		log.Fatalf("Error loading mail controller: %s", err)
		return nil
	}

	mailSvc := async.NewAsyncService(mailController, bufferSize)
	server.serviceManager.BindService(&mailSvc)

	// OAuth management module
	oauthModule := oauth.NewController(dataStore, config.OAuth)

	// Create a global context object
	server.ctx = appcontext.NewGlobalCtx(sessionStore)

	// Create router
	router := web.New(appcontext.AuthPlzCtx{}).
		Middleware(appcontext.BindContext(&server.ctx)).
		//Middleware(web.LoggerMiddleware).
		Middleware((*appcontext.AuthPlzCtx).SessionMiddleware).
		Middleware((*appcontext.AuthPlzCtx).GetIPMiddleware).
		Middleware((*appcontext.AuthPlzCtx).GetLocaleMiddleware)

	router.OptionsHandler(appcontext.NewOptionsHandler(config.AllowedOrigins))

	// Enable static file hosting
	_, _ = os.Getwd()
	staticPath := path.Clean(config.StaticDir)
	log.Printf("Loading static content from: %s\n", staticPath)
	router.Middleware(web.StaticMiddleware(staticPath))

	// Bind modules to router
	coreModule.BindAPI(router)
	userModule.BindAPI(router)
	u2fModule.BindAPI(router)
	totpModule.BindAPI(router)
	backupModule.BindAPI(router)
	auditModule.BindAPI(router)
	oauthModule.BindAPI(router)

	server.router = router

	return &server
}

// Start an instance of the AuthPlzServer
func (server *AuthPlzServer) Start() {
	// Start listening

	// Set bind address
	address := server.config.Address + ":" + server.config.Port

	// Create handlers
	contextHandler := context.ClearHandler(server.router)

	// Start async services
	server.serviceManager.Run()

	// Start with/without TLS
	var err error
	if server.config.TLS.Disabled == true {
		log.Println("*******************************************************************************")
		log.Println("WARNING: TLS IS DISABLED. USE FOR TESTING OR WITH EXTERNAL TLS TERMINATION ONLY")
		log.Println("*******************************************************************************")
		log.Printf("Listening at: http://%s", address)
		err = http.ListenAndServe(address, contextHandler)
	} else {
		log.Printf("Listening at: https://%s", address)
		err = http.ListenAndServeTLS(address, server.config.TLS.Cert, server.config.TLS.Key, contextHandler)
	}

	// Stop async services
	server.serviceManager.Exit()

	// Handle errors
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// Close an instance of the AuthPlzServer
func (server *AuthPlzServer) Close() {
	// TODO: stop HTTP server

	// Stop workers
	server.serviceManager.Exit()

	// Close datastore
	server.ds.Close()
}
