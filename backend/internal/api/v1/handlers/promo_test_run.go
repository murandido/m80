package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/promotion"
	"github.com/murandido/m-80-backend/internal/utils"
)

// TestPromoRecipe handles the POST /promo-recipes/test route
// @Summary      Testar receita de promoção
// @Description  Abre a página, tenta localizar o formulário e devolve um print, sem submeter nada. A receita vai no corpo, então dá para testar antes de salvar.
// @Tags         Promoções
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body schemas.TestPromoRecipeRequest true "Link e seletores a testar"
// @Success      200  {object}  schemas.PromoRecipeTestSingleResponse
// @Failure      400  {object}  schemas.ErrorResponse "Payload inválido"
// @Failure      401  {object}  schemas.ErrorResponse "Não autenticado"
// @Failure      403  {object}  schemas.ErrorResponse "Acesso restrito a desenvolvedores"
// @Failure      502  {object}  schemas.ErrorResponse "Serviço de promoções indisponível"
// @Router       /promo-recipes/test [post]
//
// The dry run is what makes a recipe safe to edit during a live campaign: the author
// sees exactly what the bot would see, and nothing is submitted, so a wrong selector
// costs a page load rather than an enrolment.
func (h *PromoRecipeHandler) TestPromoRecipe(w http.ResponseWriter, r *http.Request) {
	var req schemas.TestPromoRecipeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if err := h.Validator.Struct(req); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Validation failed: "+err.Error())
		return
	}
	if err := validatePatterns(req.SuccessPattern, req.FailurePattern); err != nil {
		utils.SendError(w, r, http.StatusBadRequest, "Expressão regular inválida: "+err.Error())
		return
	}

	recipe := promotion.Recipe{
		CPFSelector:      derefString(req.CPFSelector),
		CheckboxSelector: derefString(req.CheckboxSelector),
		SubmitSelector:   derefString(req.SubmitSelector),
		SuccessPattern:   derefString(req.SuccessPattern),
		FailurePattern:   derefString(req.FailurePattern),
	}
	if req.WaitSeconds != nil {
		recipe.WaitSeconds = *req.WaitSeconds
	}

	response, err := h.Promo.Enroll(r.Context(), promotion.ServiceRequest{
		URL:    req.URL,
		DryRun: true,
		Recipe: &recipe,
	})
	if err != nil {
		utils.SendError(w, r, http.StatusBadGateway, "Serviço de promoções indisponível: "+err.Error())
		return
	}

	payload := schemas.PromoRecipeTestResponse{
		Error:      response.Error,
		Screenshot: response.Screenshot,
	}
	message := "Formulário localizado"
	if response.Resolved == nil {
		// A refusal is a successful test with a negative answer, not an API error: the
		// screenshot and the reason are exactly what the author needs to fix the recipe.
		message = "Nenhum formulário de cadastro foi localizado nesta página"
	} else {
		payload.Resolved = &schemas.PromoResolutionResponse{
			CPFField: response.Resolved.CPFField,
			Checkbox: response.Resolved.Checkbox,
			Submit:   response.Resolved.Submit,
			Source:   response.Resolved.Source,
		}
	}
	utils.SendJSON(w, r, http.StatusOK, message, payload)
}
