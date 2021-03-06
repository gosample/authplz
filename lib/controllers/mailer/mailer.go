/*
 * Mailer module controller
 * This manages email sending based on system events
 *
 * AuthPlz Project (https://github.com/ryankurte/AuthPlz)
 * Copyright 2017 Ryan Kurte
 */

package mailer

import (
	"bytes"
	"fmt"
	"html/template"
	"log"

	"github.com/ryankurte/authplz/lib/api"
	"github.com/ryankurte/authplz/lib/controllers/mailer/drivers"
	"github.com/ryankurte/authplz/lib/events"
	"time"
)

// MailDriver defines the interface that must be implemented by a mailer driver
type MailDriver interface {
	Send(to, subject, body string) error
}

type Storer interface {
	GetUserByExtID(extID string) (interface{}, error)
}

type TokenCreator interface {
	BuildToken(userID string, action api.TokenAction, duration time.Duration) (string, error)
}

type User interface {
	GetUsername() string
	GetEmail() string
}

// MailController Mail controller instance
type MailController struct {
	domain       string
	appName      string
	templateDir  string
	templates    map[string]template.Template
	driver       MailDriver
	storer       Storer
	tokenCreator TokenCreator
	options      map[string]string
}

// Standard mailing templates (required for MailController creation)
var templateNames = [...]string{"activation", "passwordreset", "loginnotice"}

type MailerConfig struct {
	AppName      string
	Domain       string
	Driver       string
	Options      map[string]string
	Storer       Storer
	TokenCreator TokenCreator
	TemplateDir  string
}

// NewMailController Creates a mail controller
func NewMailController(appName, domain, driver string, options map[string]string, storer Storer, tokenCreator TokenCreator, templateDir string) (*MailController, error) {

	// Load driver
	var d MailDriver
	var err error
	switch driver {
	case drivers.MailgunDriverID:
		d, err = drivers.NewMailgunDriver(options)
	case drivers.LoggerDriverID:
		d, err = drivers.NewLoggerDriver(options)
	default:
		return nil, fmt.Errorf("NewMailController error: unrecognised driver %s", driver)
	}
	if err != nil {
		return nil, err
	}

	// Load templates from specified directory
	var templates = make(map[string]template.Template)
	for _, name := range templateNames {
		fileName := fmt.Sprintf("%s/%s.tmpl", templateDir, name)
		tpl, err := template.ParseFiles(fileName)
		if err != nil {
			return nil, err
		}
		templates[name] = *tpl
	}

	return &MailController{
		domain:       domain,
		appName:      appName,
		templateDir:  templateDir,
		templates:    templates,
		driver:       d,
		storer:       storer,
		tokenCreator: tokenCreator,
		options:      options,
	}, nil
}

// SendMail Send a message to the provided email address
func (mc *MailController) SendMail(address, subject, body string) error {
	return mc.driver.Send(address, subject, body)
}

// SendTemplate fills and sends a template based email
func (mc *MailController) SendTemplate(template, address, subject string, data map[string]string) error {
	buf := new(bytes.Buffer)
	tmpl, ok := mc.templates[template]
	if !ok {
		return fmt.Errorf("template %s not found", template)
	}
	err := tmpl.Execute(buf, data)
	if err != nil {
		return err
	}

	return mc.SendMail(address, subject, buf.String())
}

// SendActivation Send a activation email to the provided address
func (mc *MailController) SendActivation(email string, data map[string]string) error {
	return mc.SendTemplate("activation", email, mc.appName+" Account Activation", data)
}

// SendPasswordReset Send a password reset email to the provided address
func (mc *MailController) SendPasswordReset(email string, data map[string]string) error {
	return mc.SendTemplate("passwordreset", email, mc.appName+" Password Reset", data)
}

func mergeMaps(a, b map[string]string) map[string]string {
	c := make(map[string]string)
	for i := range a {
		c[i] = a[i]
	}
	for i := range b {
		c[i] = b[i]
	}
	return c
}

func (mc *MailController) HandleEvent(e interface{}) error {
	event := e.(*events.AuthPlzEvent)

	// Fetch the user object for further use
	// TODO: I wonder if we should just be passing this around to save DB accesses?
	userID := event.GetUserExtID()
	u, err := mc.storer.GetUserByExtID(userID)
	if err != nil {
		log.Printf("MailController.HandleEvent error: %s", err)
		return err
	}
	user := u.(User)

	// Fill in base data
	data := make(map[string]string)
	data["Domain"] = mc.domain
	data["ServiceName"] = mc.appName
	data["Email"] = user.GetEmail()
	data["Username"] = user.GetUsername()

	// Handle types of events
	err = nil
	switch event.GetType() {
	case events.EventAccountCreated:
		// Account creation causes an activation email to be sent
		token, err := mc.tokenCreator.BuildToken(userID, api.TokenActionActivate, time.Hour)
		if err != nil {
			log.Printf("MailController.HandleEvent error creating token %s", err)
			return err
		}
		data["Token"] = token
		data["ActionURL"] = fmt.Sprintf("%s/api/action?token=%s", mc.domain, token)
		err = mc.SendActivation(user.GetEmail(), mergeMaps(data, event.GetData()))
	case events.EventPasswordResetReq:
		// Password recovery request causes a password recovery email to be sent
		token, err := mc.tokenCreator.BuildToken(userID, api.TokenActionRecovery, time.Hour)
		if err != nil {
			log.Printf("MailController.HandleEvent error creating token %s", err)
			return err
		}
		data["Token"] = token
		data["ActionURL"] = fmt.Sprintf("%s/api/recovery?token=%s", mc.domain, token)
		err = mc.SendPasswordReset(user.GetEmail(), mergeMaps(data, event.GetData()))
	default:
	}

	return err
}
