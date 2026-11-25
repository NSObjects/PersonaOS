package boot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/samber/do/v2"
	"gorm.io/gorm"

	accessmysql "github.com/NSObjects/PersonaOS/internal/modules/access/adapters/mysql"
	accesshttp "github.com/NSObjects/PersonaOS/internal/modules/access/http"
	accessusecase "github.com/NSObjects/PersonaOS/internal/modules/access/usecase"
	agentdemomysql "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/adapters/redisrag"
	agentdemohttp "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/http"
	agentdemousecase "github.com/NSObjects/PersonaOS/internal/modules/agentdemo/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/agentdemo/voiceconfig"
	apitokenmysql "github.com/NSObjects/PersonaOS/internal/modules/apitoken/adapters/mysql"
	apitokenhttp "github.com/NSObjects/PersonaOS/internal/modules/apitoken/http"
	apitokenusecase "github.com/NSObjects/PersonaOS/internal/modules/apitoken/usecase"
	auditmysql "github.com/NSObjects/PersonaOS/internal/modules/audit/adapters/mysql"
	audithttp "github.com/NSObjects/PersonaOS/internal/modules/audit/http"
	"github.com/NSObjects/PersonaOS/internal/modules/audit/oprec"
	auditusecase "github.com/NSObjects/PersonaOS/internal/modules/audit/usecase"
	authmysql "github.com/NSObjects/PersonaOS/internal/modules/auth/adapters/mysql"
	authhttp "github.com/NSObjects/PersonaOS/internal/modules/auth/http"
	authusecase "github.com/NSObjects/PersonaOS/internal/modules/auth/usecase"
	catalogmysql "github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/mysql"
	"github.com/NSObjects/PersonaOS/internal/modules/catalog/adapters/volcclone"
	cataloghttp "github.com/NSObjects/PersonaOS/internal/modules/catalog/http"
	catalogusecase "github.com/NSObjects/PersonaOS/internal/modules/catalog/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/fileasset/adapters/localstorage"
	filemysql "github.com/NSObjects/PersonaOS/internal/modules/fileasset/adapters/mysql"
	filehttp "github.com/NSObjects/PersonaOS/internal/modules/fileasset/http"
	fileusecase "github.com/NSObjects/PersonaOS/internal/modules/fileasset/usecase"
	identitymysql "github.com/NSObjects/PersonaOS/internal/modules/identity/adapters/mysql"
	identitydomain "github.com/NSObjects/PersonaOS/internal/modules/identity/domain"
	identityhttp "github.com/NSObjects/PersonaOS/internal/modules/identity/http"
	identityusecase "github.com/NSObjects/PersonaOS/internal/modules/identity/usecase"
	"github.com/NSObjects/PersonaOS/internal/modules/providerconfig"
	settingsmysql "github.com/NSObjects/PersonaOS/internal/modules/settings/adapters/mysql"
	settingsdomain "github.com/NSObjects/PersonaOS/internal/modules/settings/domain"
	settingshttp "github.com/NSObjects/PersonaOS/internal/modules/settings/http"
	settingsusecase "github.com/NSObjects/PersonaOS/internal/modules/settings/usecase"
	setupmysql "github.com/NSObjects/PersonaOS/internal/modules/setup/adapters/mysql"
	setuphttp "github.com/NSObjects/PersonaOS/internal/modules/setup/http"
	setupusecase "github.com/NSObjects/PersonaOS/internal/modules/setup/usecase"
	"github.com/NSObjects/PersonaOS/internal/platform/configs"
	"github.com/NSObjects/PersonaOS/internal/platform/server/middlewares"
)

// BusinessModules returns the business modules installed by the default runtime.
func BusinessModules() []Module {
	return []Module{
		setupModule(),
		accessModule(),
		identityModule(),
		auditModule(),
		apiTokenModule(),
		authModule(),
		settingsModule(),
		providerConfigModule(),
		fileModule(),
		catalogModule(),
		agentDemoModule(),
	}
}

func catalogModule() Module {
	return NewModule("catalog",
		Provide(newCatalogStore),
		Provide(newCatalogUsecase),
		Provide(newCatalogHandler),
		Provide(newCatalogResourceStore),
		Provide(newCatalogResourceUsecase),
		Provide(newCatalogResourceHandler),
		Provide(newCatalogCapabilityDraftStore),
		Provide(newCatalogCapabilityDraftUsecase),
		Provide(newCatalogCapabilityDraftHandler),
		Provide(newCatalogActiveConfigStore),
		Provide(newCatalogActiveConfigUsecase),
		Provide(newCatalogActiveConfigHandler),
		Route(cataloghttp.Register),
		Route(cataloghttp.RegisterDrafts),
		Route(cataloghttp.RegisterResources),
		Route(cataloghttp.RegisterActiveConfigs),
	)
}

func newCatalogStore(i do.Injector) (*catalogmysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return catalogmysql.NewStore(ctx, db)
}

func newCatalogUsecase(i do.Injector) (*catalogusecase.Usecase, error) {
	store, err := do.Invoke[*catalogmysql.Store](i)
	if err != nil {
		return nil, err
	}
	return catalogusecase.New(store), nil
}

func newCatalogHandler(i do.Injector) (*cataloghttp.Handler, error) {
	uc, err := do.Invoke[*catalogusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return cataloghttp.New(uc, oprec.New(audit)), nil
}

func newCatalogResourceStore(i do.Injector) (*catalogmysql.ResourceStore, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return catalogmysql.NewResourceStore(ctx, db)
}

func newCatalogResourceUsecase(i do.Injector) (*catalogusecase.ResourceUsecase, error) {
	store, err := do.Invoke[*catalogmysql.ResourceStore](i)
	if err != nil {
		return nil, err
	}
	uc := catalogusecase.NewResourceUsecase(store)
	if key := os.Getenv("VOLCENGINE_API_KEY"); key != "" {
		uc.WithVoiceCloner(volcclone.New(key))
	}
	indexer, err := newDemoKnowledgeStore(i)
	if err != nil {
		return nil, err
	}
	if indexer != nil {
		uc.WithKnowledgeImport(store, indexer).WithKnowledgeInspector(indexer)
	}
	return uc, nil
}

func newCatalogResourceHandler(i do.Injector) (*cataloghttp.ResourceHandler, error) {
	uc, err := do.Invoke[*catalogusecase.ResourceUsecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return cataloghttp.NewResourceHandler(uc, oprec.New(audit)), nil
}

func newCatalogCapabilityDraftStore(i do.Injector) (*catalogmysql.CapabilityDraftStore, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	if _, err := do.Invoke[*catalogmysql.Store](i); err != nil {
		return nil, err
	}
	if _, err := do.Invoke[*catalogmysql.ResourceStore](i); err != nil {
		return nil, err
	}
	return catalogmysql.NewCapabilityDraftStore(ctx, db)
}

func newCatalogCapabilityDraftUsecase(i do.Injector) (*catalogusecase.CapabilityDraftUsecase, error) {
	store, err := do.Invoke[*catalogmysql.CapabilityDraftStore](i)
	if err != nil {
		return nil, err
	}
	return catalogusecase.NewCapabilityDraftUsecase(store), nil
}

func newCatalogCapabilityDraftHandler(i do.Injector) (*cataloghttp.DraftHandler, error) {
	uc, err := do.Invoke[*catalogusecase.CapabilityDraftUsecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return cataloghttp.NewDraftHandler(uc, oprec.New(audit)), nil
}

func newCatalogActiveConfigStore(i do.Injector) (*catalogmysql.ActiveConfigStore, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	if _, err := do.Invoke[*catalogmysql.CapabilityDraftStore](i); err != nil {
		return nil, err
	}
	return catalogmysql.NewActiveConfigStore(ctx, db)
}

func newCatalogActiveConfigUsecase(i do.Injector) (*catalogusecase.ActiveConfigUsecase, error) {
	store, err := do.Invoke[*catalogmysql.ActiveConfigStore](i)
	if err != nil {
		return nil, err
	}
	return catalogusecase.NewActiveConfigUsecase(store), nil
}

func newCatalogActiveConfigHandler(i do.Injector) (*cataloghttp.ActiveConfigHandler, error) {
	uc, err := do.Invoke[*catalogusecase.ActiveConfigUsecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return cataloghttp.NewActiveConfigHandler(uc, oprec.New(audit)), nil
}

func agentDemoModule() Module {
	return NewModule("agentdemo",
		Provide(newAgentDemoMemoryStore),
		Provide(newAgentDemoUsecase),
		Provide(newAgentDemoSessions),
		Provide(newAgentDemoHandler),
		Provide(newAgentDemoVoiceUsecase),
		Provide(newAgentDemoVoiceHandler),
		Route(agentdemohttp.Register),
		Route(agentdemohttp.RegisterVoice),
	)
}

func newAgentDemoMemoryStore(i do.Injector) (*agentdemomysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return agentdemomysql.NewStore(ctx, db)
}

func newAgentDemoUsecase(i do.Injector) (*agentdemousecase.ChatUsecase, error) {
	config, err := do.Invoke[*catalogusecase.ActiveConfigUsecase](i)
	if err != nil {
		return nil, err
	}
	memoryStore, err := do.Invoke[*agentdemomysql.Store](i)
	if err != nil {
		return nil, err
	}
	endpoints := make(map[string]string)
	if raw := os.Getenv("PERSONAOS_HTTP_TOOL_ENDPOINTS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &endpoints); err != nil {
			return nil, fmt.Errorf("invalid HTTP tool endpoint configuration: %w", err)
		}
	}
	registry := agentdemousecase.NewToolRegistry(endpoints).WithNetworkTools(
		os.Getenv("PERSONAOS_OPEN_METEO_DEMO") == "1",
		os.Getenv("DEEPSEEK_API_KEY"),
	)
	key, modelName := os.Getenv("PERSONAOS_LLM_API_KEY"), os.Getenv("PERSONAOS_LLM_MODEL")
	if key == "" || modelName == "" {
		return agentdemousecase.NewChatUsecase(config, nil).
			WithMemory(agentdemousecase.NewMemoryService(memoryStore, nil)).
			WithToolRegistry(registry), nil
	}
	chatModel, err := openai.NewChatModel(context.Background(), &openai.ChatModelConfig{
		APIKey: key, Model: modelName, BaseURL: os.Getenv("PERSONAOS_LLM_BASE_URL"),
		Timeout: 40 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	knowledge, err := newDemoKnowledgeStore(i)
	if err != nil {
		return nil, err
	}
	uc := agentdemousecase.NewChatUsecase(config, chatModel).
		WithMemory(agentdemousecase.NewMemoryService(memoryStore, chatModel)).
		WithToolRegistry(registry)
	if knowledge != nil {
		uc.WithKnowledgeRetriever(knowledge)
	}
	return uc, nil
}

func newDemoKnowledgeStore(i do.Injector) (*redisrag.Store, error) {
	key := os.Getenv("PERSONAOS_EMBEDDING_API_KEY")
	if key == "" {
		return nil, nil
	}
	ctx, err := do.Invoke[context.Context](i)
	if err != nil {
		return nil, err
	}
	bundle, err := do.Invoke[*Resources](i)
	if err != nil {
		return nil, err
	}
	client, ok := bundle.Redis()
	if !ok {
		return nil, fmt.Errorf("knowledge retrieval requires enabled Redis")
	}
	embedder, err := redisrag.NewArkEmbedder(ctx, key)
	if err != nil {
		return nil, err
	}
	return redisrag.New(ctx, client, embedder)
}

func newAgentDemoVoiceUsecase(i do.Injector) (*agentdemousecase.VoiceUsecase, error) {
	config, err := do.Invoke[*catalogusecase.ActiveConfigUsecase](i)
	if err != nil {
		return nil, err
	}
	recognizer, err := voiceconfig.NewASRFromEnv()
	if err != nil {
		return nil, err
	}
	resources, err := do.Invoke[*catalogusecase.ResourceUsecase](i)
	if err != nil {
		return nil, err
	}
	return agentdemousecase.NewVoiceUsecase(config, recognizer, voiceconfig.NewTTSFromEnv, voiceconfig.ASRInputFormat()).
		WithVoiceResources(resources), nil
}

func newAgentDemoVoiceHandler(i do.Injector) (*agentdemohttp.VoiceHandler, error) {
	voice, err := do.Invoke[*agentdemousecase.VoiceUsecase](i)
	if err != nil {
		return nil, err
	}
	return agentdemohttp.NewVoiceHandler(voice), nil
}

func newAgentDemoSessions(_ do.Injector) (*agentdemousecase.ConversationSessions, error) {
	return agentdemousecase.NewConversationSessions(), nil
}

func newAgentDemoHandler(i do.Injector) (*agentdemohttp.Handler, error) {
	chat, err := do.Invoke[*agentdemousecase.ChatUsecase](i)
	if err != nil {
		return nil, err
	}
	voice, err := do.Invoke[*agentdemousecase.VoiceUsecase](i)
	if err != nil {
		return nil, err
	}
	sessions, err := do.Invoke[*agentdemousecase.ConversationSessions](i)
	if err != nil {
		return nil, err
	}
	return agentdemohttp.New(chat, voice, sessions), nil
}

func setupModule() Module {
	return NewModule("setup",
		Provide(newSetupStore),
		Provide(newSetupTransactionRunner),
		Provide(newSetupUsecase),
		Provide(newInstallationStateReader),
		Provide(newSetupHandler),
		Route(setuphttp.Register),
	)
}

func accessModule() Module {
	return NewModule("access",
		Provide(newAccessStore),
		Provide(newAccessUsecase),
		Provide(newRouteAuthorizer),
		Provide(newAccessHandler),
		Route(accesshttp.Register),
	)
}

func apiTokenModule() Module {
	return NewModule("apitoken",
		Provide(newAPITokenStore),
		Provide(newAPITokenUsecase),
		Provide(newAPIKeyVerifier),
		Provide(newAPITokenHandler),
		Route(apitokenhttp.Register),
	)
}

func identityModule() Module {
	return NewModule("identity",
		Provide(newIdentityStore),
		Provide(newIdentityUsecase),
		Provide(newIdentityHandler),
		Route(identityhttp.Register),
	)
}

func auditModule() Module {
	return NewModule("audit",
		Provide(newAuditStore),
		Provide(newAuditUsecase),
		Provide(newSystemErrorRecorder),
		Provide(newAuditHandler),
		Route(audithttp.Register),
	)
}

func authModule() Module {
	return NewModule("auth",
		Provide(newAuthStore),
		Provide(newAuthUsecase),
		Provide(newLoginSessionAuthenticator),
		Provide(newAuthHandler),
		Route(authhttp.Register),
	)
}

func settingsModule() Module {
	return NewModule("settings",
		Provide(newSettingsStore),
		Provide(newSettingsImportRunner),
		Provide(newSettingsUsecase),
		Provide(newSettingsHandler),
		Route(settingshttp.Register),
	)
}

func providerConfigModule() Module {
	return NewModule("providerconfig",
		Provide(newProviderConfigStore),
		Provide(newProviderConfigHandler),
		Route(providerconfig.Register),
	)
}

func newProviderConfigStore(i do.Injector) (*providerconfig.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	var cipher *providerconfig.Cipher
	if key := os.Getenv("PERSONAOS_PROVIDER_MASTER_KEY"); key != "" {
		cipher, err = providerconfig.NewCipher(key)
		if err != nil {
			return nil, err
		}
	}
	return providerconfig.NewStore(ctx, db, cipher)
}

func newProviderConfigHandler(i do.Injector) (*providerconfig.Handler, error) {
	store, err := do.Invoke[*providerconfig.Store](i)
	if err != nil {
		return nil, err
	}
	roles, err := do.Invoke[*accessmysql.Store](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return providerconfig.NewHandler(store, func(ctx context.Context, id int64) (bool, error) {
		role, err := roles.FindRoleByID(ctx, id)
		return err == nil && role.Active && role.IsSuperAdmin(), err
	}, oprec.New(audit)), nil
}

func fileModule() Module {
	return NewModule("fileasset",
		Provide(newFileStore),
		Provide(newFileUsecase),
		Provide(newFileHandler),
		Route(filehttp.Register),
	)
}

func newAccessStore(i do.Injector) (*accessmysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return accessmysql.NewStore(ctx, db)
}

func newSetupStore(i do.Injector) (*setupmysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return setupmysql.NewStore(ctx, db)
}

func newSetupTransactionRunner(i do.Injector) (setupusecase.TransactionRunner, error) {
	_, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	setupStore, err := do.Invoke[*setupmysql.Store](i)
	if err != nil {
		return nil, err
	}
	accessStore, err := do.Invoke[*accessmysql.Store](i)
	if err != nil {
		return nil, err
	}
	identityStore, err := do.Invoke[*identitymysql.Store](i)
	if err != nil {
		return nil, err
	}
	settingsStore, err := do.Invoke[*settingsmysql.Store](i)
	if err != nil {
		return nil, err
	}
	return setupTransactionRunner{
		db:       db,
		setup:    setupStore,
		access:   accessStore,
		identity: identityStore,
		settings: settingsStore,
	}, nil
}

func newSetupUsecase(i do.Injector) (*setupusecase.Usecase, error) {
	store, err := do.Invoke[*setupmysql.Store](i)
	if err != nil {
		return nil, err
	}
	runner, err := do.Invoke[setupusecase.TransactionRunner](i)
	if err != nil {
		return nil, err
	}
	return setupusecase.New(store, runner), nil
}

func newInstallationStateReader(i do.Injector) (middlewares.InstallationStateReader, error) {
	uc, err := do.Invoke[*setupusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return installationStateReader{setup: uc}, nil
}

func newSetupHandler(i do.Injector) (*setuphttp.Handler, error) {
	uc, err := do.Invoke[*setupusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return setuphttp.New(uc), nil
}

func newAccessUsecase(i do.Injector) (*accessusecase.Usecase, error) {
	store, err := do.Invoke[*accessmysql.Store](i)
	if err != nil {
		return nil, err
	}
	identityStore, err := do.Invoke[*identitymysql.Store](i)
	if err != nil {
		return nil, err
	}
	return accessusecase.New(store, accessAdminRoleReader{store: identityStore}), nil
}

func newAccessHandler(i do.Injector) (*accesshttp.Handler, error) {
	uc, audit, err := accessHandlerDeps(i)
	if err != nil {
		return nil, err
	}
	return accesshttp.New(uc, oprec.New(audit)), nil
}

func newIdentityStore(i do.Injector) (*identitymysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return identitymysql.NewStore(ctx, db)
}

func newIdentityUsecase(i do.Injector) (*identityusecase.Usecase, error) {
	store, err := do.Invoke[*identitymysql.Store](i)
	if err != nil {
		return nil, err
	}
	roles, err := do.Invoke[*accessusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	auth, err := do.Invoke[*authusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return identityusecase.New(store, roles, identitySessionRevoker{auth: auth}), nil
}

func newIdentityHandler(i do.Injector) (*identityhttp.Handler, error) {
	uc, err := do.Invoke[*identityusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return identityhttp.New(uc, oprec.New(audit)), nil
}

func newAuditStore(i do.Injector) (*auditmysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return auditmysql.NewStore(ctx, db)
}

func newAuditUsecase(i do.Injector) (*auditusecase.Usecase, error) {
	store, err := do.Invoke[*auditmysql.Store](i)
	if err != nil {
		return nil, err
	}
	return auditusecase.New(store), nil
}

func newSystemErrorRecorder(i do.Injector) (middlewares.SystemErrorRecorder, error) {
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return systemErrorRecorder{audit: audit}, nil
}

func newAuditHandler(i do.Injector) (*audithttp.Handler, error) {
	uc, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return audithttp.New(uc), nil
}

func newAPITokenStore(i do.Injector) (*apitokenmysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return apitokenmysql.NewStore(ctx, db)
}

func newAPITokenUsecase(i do.Injector) (*apitokenusecase.Usecase, error) {
	store, err := do.Invoke[*apitokenmysql.Store](i)
	if err != nil {
		return nil, err
	}
	identityStore, err := do.Invoke[*identitymysql.Store](i)
	if err != nil {
		return nil, err
	}
	accessStore, err := do.Invoke[*accessmysql.Store](i)
	if err != nil {
		return nil, err
	}
	return apitokenusecase.New(
		store,
		apiTokenAdminReader{store: identityStore},
		apiTokenRolePolicy{store: accessStore},
	), nil
}

func newAPIKeyVerifier(i do.Injector) (middlewares.APIKeyVerifier, error) {
	uc, err := do.Invoke[*apitokenusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return apiKeyVerifier{tokens: uc}, nil
}

func newAPITokenHandler(i do.Injector) (*apitokenhttp.Handler, error) {
	uc, err := do.Invoke[*apitokenusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return apitokenhttp.New(uc, oprec.New(audit)), nil
}

func newAuthUsecase(i do.Injector) (*authusecase.Usecase, error) {
	authStore, err := do.Invoke[*authmysql.Store](i)
	if err != nil {
		return nil, err
	}
	identityStore, err := do.Invoke[*identitymysql.Store](i)
	if err != nil {
		return nil, err
	}
	authorization, err := do.Invoke[*accessusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return authusecase.New(identityStore, authAuthorizationReader{access: authorization}, authStore, authStore, authLoginRecorder{audit: audit}), nil
}

func newAuthStore(i do.Injector) (*authmysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return authmysql.NewStore(ctx, db)
}

func newLoginSessionAuthenticator(i do.Injector) (middlewares.LoginSessionAuthenticator, error) {
	uc, err := do.Invoke[*authusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return loginSessionAuthenticator{auth: uc}, nil
}

type loginSessionAuthenticator struct {
	auth *authusecase.Usecase
}

func (a loginSessionAuthenticator) AuthenticateLoginSession(ctx context.Context, token string) (middlewares.LoginSessionIdentity, error) {
	identity, err := a.auth.AuthenticateLoginSession(ctx, token)
	if err != nil {
		return middlewares.LoginSessionIdentity{}, err
	}
	return middlewares.LoginSessionIdentity{
		SessionID: identity.SessionID,
		UserID:    identity.AdminID,
		RoleID:    identity.RoleID,
	}, nil
}

type identitySessionRevoker struct {
	auth *authusecase.Usecase
}

func (r identitySessionRevoker) RevokeLoginSessions(ctx context.Context, adminID int64) error {
	return r.auth.RevokeLoginSessions(ctx, adminID)
}

type apiKeyVerifier struct {
	tokens *apitokenusecase.Usecase
}

func (v apiKeyVerifier) VerifyAPIKey(ctx context.Context, secret string) (middlewares.APIKeyIdentity, error) {
	identity, err := v.tokens.Authenticate(ctx, secret)
	if err != nil {
		return middlewares.APIKeyIdentity{}, err
	}
	return middlewares.APIKeyIdentity{
		UserID: identity.AdminID,
		RoleID: identity.RoleID,
	}, nil
}

type systemErrorRecorder struct {
	audit *auditusecase.Usecase
}

func (r systemErrorRecorder) RecordSystemError(ctx context.Context, input middlewares.SystemErrorInput) error {
	_, err := r.audit.RecordSystemError(ctx, auditusecase.SystemErrorInput{
		Code:      input.Code,
		Message:   input.Message,
		Detail:    input.Detail,
		Method:    input.Method,
		Path:      input.Path,
		IP:        input.IP,
		UserAgent: input.UserAgent,
		RequestID: input.RequestID,
		UserID:    input.UserID,
	})
	return err
}

func newAuthHandler(i do.Injector) (*authhttp.Handler, error) {
	uc, err := do.Invoke[*authusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	cfg, err := do.Invoke[configs.Config](i)
	if err != nil {
		return nil, err
	}
	return authhttp.New(uc, cfg.HTTP.SecureCookies), nil
}

func newSettingsStore(i do.Injector) (*settingsmysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return settingsmysql.NewStore(ctx, db)
}

func newSettingsUsecase(i do.Injector) (*settingsusecase.Usecase, error) {
	store, err := do.Invoke[*settingsmysql.Store](i)
	if err != nil {
		return nil, err
	}
	accessUC, err := do.Invoke[*accessusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	importRunner, err := do.Invoke[settingsusecase.ImportRunner](i)
	if err != nil {
		return nil, err
	}
	return settingsusecase.New(store,
		settingsusecase.WithVersionCatalog(settingsVersionCatalog{
			access: accessUC,
		}),
		settingsusecase.WithImportRunner(importRunner),
	), nil
}

func newSettingsImportRunner(i do.Injector) (settingsusecase.ImportRunner, error) {
	_, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	settingsStore, err := do.Invoke[*settingsmysql.Store](i)
	if err != nil {
		return nil, err
	}
	accessStore, err := do.Invoke[*accessmysql.Store](i)
	if err != nil {
		return nil, err
	}
	accessUC, err := do.Invoke[*accessusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return settingsImportRunner{
		db:       db,
		settings: settingsStore,
		access:   accessStore,
		accessUC: accessUC,
	}, nil
}

func newSettingsHandler(i do.Injector) (*settingshttp.Handler, error) {
	uc, err := do.Invoke[*settingsusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return settingshttp.New(uc, oprec.New(audit)), nil
}

func newFileStore(i do.Injector) (*filemysql.Store, error) {
	ctx, db, err := startupMySQL(i)
	if err != nil {
		return nil, err
	}
	return filemysql.NewStore(ctx, db)
}

func newFileUsecase(i do.Injector) (*fileusecase.Usecase, error) {
	store, err := do.Invoke[*filemysql.Store](i)
	if err != nil {
		return nil, err
	}
	cfg, err := do.Invoke[configs.Config](i)
	if err != nil {
		return nil, err
	}
	return fileusecase.New(store, localstorage.New(cfg.Admin.UploadDir)), nil
}

func newFileHandler(i do.Injector) (*filehttp.Handler, error) {
	uc, err := do.Invoke[*fileusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, err
	}
	return filehttp.New(uc, oprec.New(audit)), nil
}

func startupMySQL(i do.Injector) (context.Context, *gorm.DB, error) {
	ctx, err := do.Invoke[context.Context](i)
	if err != nil {
		return nil, nil, err
	}
	db, err := do.Invoke[*gorm.DB](i)
	if err != nil {
		return nil, nil, err
	}
	return ctx, db, nil
}

func accessHandlerDeps(i do.Injector) (*accessusecase.Usecase, *auditusecase.Usecase, error) {
	uc, err := do.Invoke[*accessusecase.Usecase](i)
	if err != nil {
		return nil, nil, err
	}
	audit, err := do.Invoke[*auditusecase.Usecase](i)
	if err != nil {
		return nil, nil, err
	}
	return uc, audit, nil
}

type authLoginRecorder struct {
	audit *auditusecase.Usecase
}

type installationStateReader struct {
	setup *setupusecase.Usecase
}

func (r installationStateReader) Initialized(ctx context.Context) (bool, error) {
	state, err := r.setup.State(ctx)
	if err != nil {
		return false, err
	}
	return state.Initialized, nil
}

type setupTransactionRunner struct {
	db       *gorm.DB
	setup    *setupmysql.Store
	access   *accessmysql.Store
	identity *identitymysql.Store
	settings *settingsmysql.Store
}

func (r setupTransactionRunner) RunInitialization(ctx context.Context, fn func(context.Context, setupusecase.Transaction) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		transaction := setupTransaction{
			setup:    r.setup.WithDB(tx),
			access:   r.access.WithDB(tx),
			identity: r.identity.WithDB(tx),
			settings: r.settings.WithDB(tx),
		}
		return fn(ctx, transaction)
	})
}

type setupTransaction struct {
	setup    *setupmysql.Store
	access   *accessmysql.Store
	identity *identitymysql.Store
	settings *settingsmysql.Store
}

func (t setupTransaction) RequireOpenInstallation(ctx context.Context) error {
	return t.setup.RequireOpenInstallation(ctx)
}

func (t setupTransaction) InstallRootAuthorization(ctx context.Context) (setupusecase.RootRole, error) {
	role, err := t.access.InstallRootAuthorization(ctx)
	if err != nil {
		return setupusecase.RootRole{}, err
	}
	return setupusecase.RootRole{ID: role.ID, Code: role.Code}, nil
}

func (t setupTransaction) CreateFirstAdministrator(ctx context.Context, input setupusecase.FirstAdministrator) error {
	admin, err := identitydomain.NewFirstAdministrator(input.Username, input.DisplayName, input.Email, input.PasswordHash, input.RootRoleID)
	if err != nil {
		return err
	}
	_, err = t.identity.Create(ctx, admin)
	return err
}

func (t setupTransaction) InstallInitialSettings(ctx context.Context, input setupusecase.InitialSettings) error {
	return t.settings.InstallInitialSettings(ctx, input.SiteName)
}

func (t setupTransaction) CompleteInstallation(ctx context.Context) error {
	return t.setup.CompleteInstallation(ctx)
}

type accessAdminRoleReader struct {
	store *identitymysql.Store
}

type apiTokenAdminReader struct {
	store *identitymysql.Store
}

func (r apiTokenAdminReader) AdminSnapshot(ctx context.Context, adminID int64) (apitokenusecase.AdminSnapshot, error) {
	admin, err := r.store.FindByID(ctx, adminID)
	if err != nil {
		return apitokenusecase.AdminSnapshot{}, err
	}
	return apitokenusecase.AdminSnapshot{
		RoleIDs: admin.RoleIDs,
		Active:  admin.Active,
	}, nil
}

type apiTokenRolePolicy struct {
	store *accessmysql.Store
}

type settingsVersionCatalog struct {
	access *accessusecase.Usecase
}

// ExportVersionMenus implements settingsusecase.VersionCatalog for access menus.
// The bridge only maps types; export rules live in the access usecase.
func (c settingsVersionCatalog) ExportVersionMenus(ctx context.Context, ids []int64) ([]accessusecase.MenuTreeInput, error) {
	return c.access.ExportMenuTree(ctx, ids)
}

// settingsImportRunner runs settings imports in one database transaction. Menu
// writes go through a transaction-scoped copy of the access usecase so import
// rules stay in the owning usecase.
type settingsImportRunner struct {
	db       *gorm.DB
	settings *settingsmysql.Store
	access   *accessmysql.Store
	accessUC *accessusecase.Usecase
}

// RunImport implements settingsusecase.ImportRunner.
func (r settingsImportRunner) RunImport(ctx context.Context, fn func(context.Context, settingsusecase.ImportTransaction) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		transaction := settingsImportTransaction{
			access:   r.accessUC.WithStore(r.access.WithDB(tx)),
			settings: r.settings.WithDB(tx),
		}
		return fn(ctx, transaction)
	})
}

// settingsImportTransaction implements settingsusecase.ImportTransaction. The
// bridge only binds transaction-scoped stores; import rules live in the
// owning usecases.
type settingsImportTransaction struct {
	access   *accessusecase.Usecase
	settings *settingsmysql.Store
}

// ImportMenus implements settingsusecase.ImportTransaction for access menus.
func (t settingsImportTransaction) ImportMenus(ctx context.Context, menus []accessusecase.MenuTreeInput) error {
	return t.access.ImportMenuTree(ctx, menus)
}

// ReplaceDictionary implements settingsusecase.ImportTransaction for dictionaries.
func (t settingsImportTransaction) ReplaceDictionary(ctx context.Context, dictionary settingsdomain.Dictionary) error {
	_, err := t.settings.ReplaceDictionary(ctx, dictionary)
	return err
}

// CreateVersion implements settingsusecase.ImportTransaction for release records.
func (t settingsImportTransaction) CreateVersion(ctx context.Context, version settingsdomain.SystemVersion) (settingsdomain.SystemVersion, error) {
	return t.settings.CreateVersion(ctx, version)
}

// RoleView implements apitokenusecase.RolePolicy for access roles. The bridge
// only projects role state; policy decisions live in the apitoken usecase.
func (p apiTokenRolePolicy) RoleView(ctx context.Context, roleID int64) (apitokenusecase.RoleView, error) {
	role, err := p.store.FindRoleByID(ctx, roleID)
	if err != nil {
		return apitokenusecase.RoleView{}, err
	}
	return apitokenusecase.RoleView{IsSuper: role.IsSuperAdmin()}, nil
}

func (r accessAdminRoleReader) AdminRoleState(ctx context.Context, adminID int64) (accessusecase.AdminRoleState, error) {
	admin, err := r.store.FindByID(ctx, adminID)
	if err != nil {
		return accessusecase.AdminRoleState{}, err
	}
	return accessusecase.AdminRoleState{
		RoleIDs:      admin.RoleIDs,
		ActiveRoleID: admin.ActiveRoleID,
		Active:       admin.Active,
	}, nil
}

func (r accessAdminRoleReader) RoleAssigned(ctx context.Context, roleID int64) (bool, error) {
	return r.store.RoleAssigned(ctx, roleID)
}

func (r authLoginRecorder) RecordLogin(ctx context.Context, record authusecase.LoginRecord) error {
	_, err := r.audit.RecordLogin(ctx, auditusecase.LoginInput{
		AdminID:   record.AdminID,
		Username:  record.Username,
		IP:        record.IP,
		UserAgent: record.UserAgent,
		Success:   record.Success,
		Reason:    record.Reason,
	})
	return err
}
