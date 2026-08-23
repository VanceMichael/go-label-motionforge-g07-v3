package httpapi

import (
	"net/http"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func (a *API) bootstrap(writer http.ResponseWriter, request *http.Request) {
	var input auth.BootstrapInput
	if err := decodeJSON(writer, request, &input); err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	tenant, user, err := a.auth.Bootstrap(request.Context(), input)
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	user.PasswordHash = ""
	writeJSON(writer, http.StatusCreated, map[string]any{"tenant": tenant, "user": user})
}

func (a *API) login(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		TenantID string `json:"tenant_id"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	result, err := a.auth.Login(request.Context(), auth.LoginInput{TenantID: input.TenantID, Email: input.Email, Password: input.Password, RequestID: requestIDFromContext(request.Context())})
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (a *API) logout(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	if err == nil {
		err = a.auth.Logout(request.Context(), principal, requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "revoked"})
}

func (a *API) createUser(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Email       string      `json:"email"`
		DisplayName string      `json:"display_name"`
		Password    string      `json:"password"`
		Role        domain.Role `json:"role"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var user domain.User
	if err == nil {
		user, err = a.auth.CreateUser(request.Context(), principal, input.Email, input.DisplayName, input.Password, input.Role, requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	user.PasswordHash = ""
	writeJSON(writer, http.StatusCreated, user)
}

func (a *API) createFacility(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Name     string `json:"name"`
		Timezone string `json:"timezone"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.Facility
	if err == nil {
		value, err = a.facilities.CreateFacility(request.Context(), principal, input.Name, input.Timezone, requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, value)
}

func (a *API) createRig(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		FacilityID   string               `json:"facility_id"`
		Name         string               `json:"name"`
		Capabilities domain.RigCapability `json:"capabilities"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.CaptureRig
	if err == nil {
		value, err = a.facilities.CreateRig(request.Context(), principal, input.FacilityID, input.Name, input.Capabilities, requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, value)
}

func (a *API) createScenario(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Name        string               `json:"name"`
		Environment string               `json:"environment"`
		Required    domain.RigCapability `json:"required_capabilities"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.Scenario
	if err == nil {
		value, err = a.facilities.CreateScenario(request.Context(), principal, input.Name, input.Environment, input.Required, requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, value)
}
