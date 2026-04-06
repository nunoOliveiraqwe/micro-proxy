package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/middleware"
	"github.com/nunoOliveiraqwe/torii/proxy/acme"
	"go.uber.org/zap"
)

func handleGetAcmeProviders(_ app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		factories := acme.ListDNSProviders()
		resp := make([]AcmeProviderResponse, 0, len(factories))
		for _, f := range factories {
			provider, err := acme.GetDNSProvider(f)
			if err != nil {
				zap.S().Warnf("Failed to get DNS-01 provider for string %s", f)
				continue
			}
			fields := make([]AcmeProviderField, 0, len(provider.Fields()))
			for _, pf := range provider.Fields() {
				fields = append(fields, AcmeProviderField{
					Key:         pf.Key,
					Label:       pf.Label,
					Required:    pf.Required,
					Sensitive:   pf.Sensitive,
					Placeholder: pf.Placeholder,
				})
			}
			resp = append(resp, AcmeProviderResponse{Name: f, Fields: fields})
		}
		WriteResponseAsJSON(resp, w)
	}
}

func handleGetAcmeConfig(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching ACME configuration")

		result, err := svc.GetServiceStore().GetAcmeService().GetConfiguration()
		if err != nil {
			logger.Error("Failed to fetch ACME configuration", zap.Error(err))
			http.Error(w, "Failed to fetch ACME configuration", http.StatusInternalServerError)
			return
		}

		WriteResponseAsJSON(AcmeConfigResponse{
			Email:                result.Email,
			DNSProvider:          result.DNSProvider,
			CADirURL:             result.CADirURL,
			RenewalCheckInterval: result.RenewalCheckInterval,
			Enabled:              result.Enabled,
			Configured:           result.Configured,
		}, w)
	}
}

func handleSaveAcmeConfig(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Saving ACME configuration")

		req, err := DecodeJSONBody[AcmeConfigRequest](r)
		if err != nil {
			logger.Error("Invalid request body", zap.Error(err))
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		dnsProvider := ""
		var credMap map[string]string
		if req.DnsProviderConfigRequest != nil {
			dnsProvider = req.DnsProviderConfigRequest.DNSProvider
			credMap = req.DnsProviderConfigRequest.ConfigMap
		}

		err = svc.GetServiceStore().GetAcmeService().SaveConfiguration(&app.SaveAcmeConfigRequest{
			Email:                req.Email,
			CADirURL:             req.CADirURL,
			RenewalCheckInterval: req.RenewalCheckInterval,
			Enabled:              req.Enabled,
			DNSProvider:          dnsProvider,
			CredentialMap:        credMap,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, app.ErrAcmeAlreadyConfigured) {
				status = http.StatusConflict
			} else if errors.Is(err, app.ErrEmailRequired) ||
				errors.Is(err, app.ErrDNSProviderRequired) ||
				errors.Is(err, app.ErrInvalidDNSProvider) ||
				errors.Is(err, app.ErrInvalidDNSProviderCfg) ||
				errors.Is(err, app.ErrInvalidRenewalFmt) ||
				errors.Is(err, app.ErrRenewalTooShort) {
				status = http.StatusBadRequest
			}
			logger.Error("Failed to save ACME configuration", zap.Error(err))
			http.Error(w, err.Error(), status)
			return
		}
	}
}

func handleGetAcmeCertificates(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching ACME certificates")

		certs, err := svc.GetServiceStore().GetAcmeService().ListCertificates()
		if err != nil {
			logger.Error("Failed to fetch ACME certificates", zap.Error(err))
			http.Error(w, "Failed to fetch ACME certificates", http.StatusInternalServerError)
			return
		}

		resp := make([]AcmeCertificateResponse, 0, len(certs))
		for _, c := range certs {
			resp = append(resp, AcmeCertificateResponse{
				Domain:    c.Domain,
				ExpiresAt: c.ExpiresAt.Format(time.RFC3339),
				CreatedAt: c.CreatedAt.Format(time.RFC3339),
				Active:    c.Active,
			})
		}
		WriteResponseAsJSON(resp, w)
	}
}

func handleToggleAcmeEnabled(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)

		req, err := DecodeJSONBody[AcmeToggleRequest](r)
		if err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := svc.GetServiceStore().GetAcmeService().ToggleEnabled(req.Enabled); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, app.ErrAcmeNotConfigured) {
				status = http.StatusNotFound
			}
			logger.Error("Failed to toggle ACME enabled state", zap.Error(err))
			http.Error(w, err.Error(), status)
			return
		}

		state := "disabled"
		if req.Enabled {
			state = "enabled"
		}
		logger.Info("ACME " + state)
		WriteResponseAsJSON(map[string]string{"status": "ACME " + state + " successfully."}, w)
	}
}

func handleResetAcme(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Warn("Resetting all ACME data")

		if err := svc.GetServiceStore().GetAcmeService().ResetAll(); err != nil {
			logger.Error("Failed to reset ACME data", zap.Error(err))
			http.Error(w, "Failed to reset ACME data", http.StatusInternalServerError)
			return
		}

		logger.Info("ACME data reset successfully")
		w.WriteHeader(http.StatusNoContent)
	}
}
