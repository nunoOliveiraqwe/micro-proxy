package api

import (
	"net/http"
	"time"

	"github.com/go-acme/lego/v4/log"
	"github.com/nunoOliveiraqwe/torii/internal/app"
	"github.com/nunoOliveiraqwe/torii/internal/domain"
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
				log.Warnf("Failed to get DNS-01 provider for string %s", f)
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

		conf, err := svc.GetServiceStore().GetAcmeStore().GetConfiguration()
		if err != nil {
			logger.Error("Failed to fetch ACME configuration", zap.Error(err))
			http.Error(w, "Failed to fetch ACME configuration", http.StatusInternalServerError)
			return
		}
		if conf == nil {
			WriteResponseAsJSON(AcmeConfigResponse{Enabled: false}, w)
			return
		}

		WriteResponseAsJSON(AcmeConfigResponse{
			Email:                conf.Email,
			DNSProvider:          conf.DNSProvider,
			CADirURL:             conf.CADirURL,
			RenewalCheckInterval: conf.RenewalCheckInterval.String(),
			Enabled:              conf.Enabled,
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

		if req.Email == "" {
			http.Error(w, "Email is required", http.StatusBadRequest)
			return
		}
		if req.DnsProviderConfigRequest == nil {
			http.Error(w, "DNS provider is required", http.StatusBadRequest)
			return
		} else if req.DnsProviderConfigRequest.DNSProvider == "" {
			http.Error(w, "DNS provider name is required", http.StatusBadRequest)
			return
		}

		provider, err := acme.GetDNSProvider(req.DnsProviderConfigRequest.DNSProvider)
		if err != nil {
			logger.Warn("Failed to get DNS-01 provider for string %s", zap.String("dns_provider", req.DnsProviderConfigRequest.DNSProvider))
			http.Error(w, "Invalid DNS provider", http.StatusBadRequest)
			return
		}

		err = provider.IsValidMap(req.DnsProviderConfigRequest.ConfigMap)
		if err != nil {
			logger.Warn("Invalid configuration for DNS provider %s: %s", zap.String("dns_provider", req.DnsProviderConfigRequest.DNSProvider))
			http.Error(w, "Invalid DNS provider configuration", http.StatusBadRequest)
			return
		}

		renewalInterval := 12 * time.Hour
		if req.RenewalCheckInterval != "" {
			parsed, err := time.ParseDuration(req.RenewalCheckInterval)
			if err != nil {
				http.Error(w, "Invalid renewal interval format (use Go duration, e.g. 12h, 6h30m)", http.StatusBadRequest)
				return
			}
			if parsed < 1*time.Hour {
				http.Error(w, "Renewal interval must be at least 1h", http.StatusBadRequest)
				return
			}
			renewalInterval = parsed
		}

		sf, err := provider.Serialize(req.DnsProviderConfigRequest.ConfigMap)
		if err != nil {
			log.Warnf("Failed to serialize configuration for DNS provider %s: %s", req.DnsProviderConfigRequest.DNSProvider, err)
			http.Error(w, "Failed to serialize DNS provider configuration", http.StatusInternalServerError)
			return
		}

		conf := &domain.AcmeConfiguration{
			Email:                req.Email,
			DNSProvider:          provider.Name(),
			CADirURL:             req.CADirURL,
			RenewalCheckInterval: renewalInterval,
			Enabled:              req.Enabled,
			SerializedFields:     sf,
		}

		if err := svc.GetServiceStore().GetAcmeStore().SaveConfiguration(conf); err != nil {
			logger.Error("Failed to save ACME configuration", zap.Error(err))
			http.Error(w, "Failed to save ACME configuration", http.StatusInternalServerError)
			return
		}

		if err := svc.ReloadAcme(); err != nil {
			logger.Error("ACME configuration saved but reload failed", zap.Error(err))
			http.Error(w, "Configuration saved but failed to apply: "+err.Error(), http.StatusInternalServerError)
			return
		}

		logger.Info("ACME configuration saved and applied",
			zap.String("email", req.Email),
			zap.String("dnsProvider", provider.Name()),
			zap.Bool("enabled", req.Enabled),
		)
	}
}

func handleGetAcmeCertificates(svc app.SystemService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := middleware.GetRequestLoggerFromContext(r)
		logger.Debug("Fetching ACME certificates")

		certs, err := svc.GetServiceStore().GetAcmeStore().ListCertificates()
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
			})
		}
		WriteResponseAsJSON(resp, w)
	}
}
