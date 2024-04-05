package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/henderiw-nephio/kform/kform-sdk-go/pkg/diag"
	kformschema "github.com/henderiw-nephio/kform/kform-sdk-go/pkg/schema"
	"github.com/henderiw/logger/log"
	"github.com/kform-providers/kubernetes/provider/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/cli-utils/pkg/flowcontrol"
)

func Provider() *kformschema.Provider {
	p := &kformschema.Provider{
		//Schema:         provSchema,
		ResourceMap: map[string]*kformschema.Resource{
			"kubernetes_manifest": resourceKubernetesManifest(),
		},
		DataSourcesMap: map[string]*kformschema.Resource{
			"kubernetes_manifest": dataSourceKubernetesManifest(),
		},
		ListDataSourcesMap: map[string]*kformschema.Resource{
			"kubernetes_manifest": dataSourcesKubernetesManifest(),
		},
	}
	p.ConfigureContextFunc = func(ctx context.Context, d []byte) (any, diag.Diagnostics) {
		return providerConfigure(ctx, d, p.Version)
	}
	return p
}

/*
func (k kubeClientsets) MainClientset() (*kubernetes.Clientset, error) {
	if k.mainClientset != nil {
		return k.mainClientset, nil
	}

	if k.config != nil {
		kc, err := kubernetes.NewForConfig(k.config)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure client: %s", err)
		}
		k.mainClientset = kc
	}
	return k.mainClientset, nil
}
*/

func providerConfigure(ctx context.Context, d []byte, _ string) (any, diag.Diagnostics) {
	log := log.FromContext(ctx)
	providerConfig := &v1alpha1.ProviderConfig{}
	if err := json.Unmarshal(d, providerConfig); err != nil {
		return nil, diag.FromErr(err)
	}

	kubeConfigFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	if providerConfig.Spec.ConfigPath != nil {
		kubeConfigFlags.KubeConfig = providerConfig.Spec.ConfigPath
	}
	matchVersionKubeConfigFlags := util.NewMatchVersionFlags(kubeConfigFlags)
	f := util.NewFactory(matchVersionKubeConfigFlags)

	restConfig, err := f.ToRESTConfig()
	if err != nil {
		return nil, diag.FromErr(err)
	}
	enabled, err := flowcontrol.IsEnabled(ctx, restConfig)
	if err != nil {
		return nil, diag.FromErr(fmt.Errorf("checking server-side throttling enablement: %w", err))
	}
	if enabled {
		// WrapConfigFn will affect future Factory.ToRESTConfig() calls.
		kubeConfigFlags.WrapConfigFn = func(cfg *rest.Config) *rest.Config {
			cfg.QPS = -1
			cfg.Burst = -1
			return cfg
		}
	}

	dc, err := f.DynamicClient()
	if err != nil {
		log.Error("cannot get dynamic client", "error", err.Error())
		return nil, diag.FromErr(err)
	}

	mapper, err := f.ToRESTMapper()
	if err != nil {
		log.Error("cannot get mapper", "error", err.Error())
		return nil, diag.FromErr(err)
	}

	/*
		cfg := ctrl.GetConfigOrDie()
		cfg.UserAgent = fmt.Sprintf("K8sForm/%s", version)
		c, err := k8sclient.New(k8sclient.Config{
			RESTConfig:        cfg,
			IgnoreAnnotations: []string{},
			IgnoreLabels:      []string{},
		})
		if err != nil {
			return nil, diag.FromErr(err)
		}
	*/

	/*
		if !providerConfig.Spec.IsKindValid() {
			return nil, diag.Errorf("invalid provider kind, got: %s, expected: %v", providerConfig.Kind, v1alpha1.ExpectedProviderKinds)
		}

		if providerConfig.Spec.Kind == v1alpha1.ProviderKindPackage {
			dir := "./out"
			if providerConfig.Spec.Directory != nil {
				dir = *providerConfig.Spec.Directory
			}

			c, err := pkgclient.New(pkgclient.Config{
				Dir:               dir,
				IgnoreAnnotations: []string{},
				IgnoreLabels:      []string{},
			})
			if err != nil {
				return nil, diag.FromErr(err)
			}
			return c, diag.Diagnostics{}
		}

		cfg, err := initializeConfiguration(ctx, providerConfig)
		if err != nil {
			return nil, diag.FromErr(err)
		}
		if cfg == nil {
			// IMPORTANT: if the supplied configuration is incomplete or invalid
			///IMPORTANT: provider operations will fail or attempt to connect to localhost endpoints
			cfg = &rest.Config{}
		}
		cfg.UserAgent = fmt.Sprintf("K8sForm/%s", version)

		c, err := k8sclient.New(k8sclient.Config{
			RESTCOnfig:        cfg,
			IgnoreAnnotations: []string{},
			IgnoreLabels:      []string{},
		})
		if err != nil {
			return nil, diag.FromErr(err)
		}
	*/

	return &Client{
		//f:               f,
		dc:     dc,
		mapper: mapper,
		//discoveryClient: discoveryClient,
	}, diag.Diagnostics{}
}

/*
func initializeConfiguration(_ context.Context, providerConfig *v1alpha1.ProviderConfig) (*rest.Config, error) {
	overrides := &clientcmd.ConfigOverrides{}
	loader := &clientcmd.ClientConfigLoadingRules{}

	configPaths := []string{}
	if providerConfig.Spec.ConfigPath != nil {
		configPaths = []string{*providerConfig.Spec.ConfigPath}
	} else if len(providerConfig.Spec.ConfigPaths) > 0 {
		configPaths = append(configPaths, providerConfig.Spec.ConfigPaths...)
	} else if v := os.Getenv("KUBE_CONFIG_PATHS"); v != "" {
		configPaths = filepath.SplitList(v)
	}

	if len(configPaths) > 0 && providerConfig.Spec.UseConfigFile != nil && *providerConfig.Spec.UseConfigFile {
		expandedPaths := []string{}
		for _, p := range configPaths {
			path, err := homedir.Expand(p)
			if err != nil {
				return nil, err
			}
			slog.Debug("using kubeconfig", "file", path)
			expandedPaths = append(expandedPaths, path)
		}

		if len(expandedPaths) == 1 {
			loader.ExplicitPath = expandedPaths[0]
		} else {
			loader.Precedence = expandedPaths
		}
		ctxSuffix := "; default context"

		if providerConfig.Spec.ConfigContext != nil ||
			providerConfig.Spec.ConfigContextAuthInfo != nil ||
			providerConfig.Spec.ConfigContextCluster != nil {
			ctxSuffix = "; overridden context"
			if providerConfig.Spec.ConfigContext != nil {
				overrides.CurrentContext = *providerConfig.Spec.ConfigContext
				ctxSuffix += fmt.Sprintf("; config ctx: %s", overrides.CurrentContext)
				slog.Debug("using custom current context", "context", overrides.CurrentContext)
			}
			overrides.Context = clientcmdapi.Context{}
			if providerConfig.Spec.ConfigContextAuthInfo != nil {
				overrides.Context.AuthInfo = *providerConfig.Spec.ConfigContextAuthInfo
				ctxSuffix += fmt.Sprintf("; auth_info: %s", overrides.Context.AuthInfo)
			}
			if providerConfig.Spec.ConfigContextCluster != nil {
				overrides.Context.Cluster = *providerConfig.Spec.ConfigContextCluster
				ctxSuffix += fmt.Sprintf("; cluster: %s", overrides.Context.Cluster)
			}
			slog.Debug("using overridden context", "context", overrides.Context)
		}
	}

	// Overriding with static configuration
	if providerConfig.Spec.Insecure != nil {
		overrides.ClusterInfo.InsecureSkipTLSVerify = *providerConfig.Spec.Insecure
	}
	if providerConfig.Spec.TLSServerName != nil {
		overrides.ClusterInfo.TLSServerName = *providerConfig.Spec.TLSServerName
	}
	if providerConfig.Spec.ClusterCACertificate != nil {
		overrides.ClusterInfo.CertificateAuthorityData = bytes.NewBufferString(*providerConfig.Spec.ClusterCACertificate).Bytes()
	}
	if providerConfig.Spec.ClientCertificate != nil {
		overrides.AuthInfo.ClientCertificateData = bytes.NewBufferString(*providerConfig.Spec.ClientCertificate).Bytes()
	}
	if providerConfig.Spec.Host != nil {
		// Server has to be the complete address of the kubernetes cluster (scheme://hostname:port), not just the hostname,
		// because `overrides` are processed too late to be taken into account by `defaultServerUrlFor()`.
		// This basically replicates what defaultServerUrlFor() does with config but for overrides,
		// see https://github.com/kubernetes/client-go/blob/v12.0.0/rest/url_utils.go#L85-L87
		hasCA := len(overrides.ClusterInfo.CertificateAuthorityData) != 0
		hasCert := len(overrides.AuthInfo.ClientCertificateData) != 0
		defaultTLS := hasCA || hasCert || overrides.ClusterInfo.InsecureSkipTLSVerify
		host, _, err := rest.DefaultServerURL(*providerConfig.Spec.Host, "", apimachineryschema.GroupVersion{}, defaultTLS)
		if err != nil {
			return nil, fmt.Errorf("failed to parse host: %s", err)
		}

		overrides.ClusterInfo.Server = host.String()
	}
	if providerConfig.Spec.Username != nil {
		overrides.AuthInfo.Username = *providerConfig.Spec.Username
	}
	if providerConfig.Spec.Password != nil {
		overrides.AuthInfo.Password = *providerConfig.Spec.Password
	}
	if providerConfig.Spec.ClientKey != nil {
		overrides.AuthInfo.ClientKeyData = bytes.NewBufferString(*providerConfig.Spec.ClientKey).Bytes()
	}
	if providerConfig.Spec.Token != nil {
		overrides.AuthInfo.Token = *providerConfig.Spec.Token
	}


	//	if providerConfig.Spec.Exec != nil {
	//		exec := &clientcmdapi.ExecConfig{
	//			APIVersion: providerConfig.Spec.Exec.APIVersion,
	//			Command:    providerConfig.Spec.Exec.Command,
	//			Args:       providerConfig.Spec.Exec.Args,
	//		}
	//		for k, v := range providerConfig.Spec.Exec.Env {
	//			exec.Env = append(exec.Env, clientcmdapi.ExecEnvVar{Name: k, Value: v})
	//		}
	//		overrides.AuthInfo.Exec = exec
	//	}


	if providerConfig.Spec.ProxyURL != nil {
		overrides.ClusterDefaults.ProxyURL = *providerConfig.Spec.ProxyURL
	}

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, overrides)
	cfg, err := cc.ClientConfig()
	if err != nil {
		slog.Warn("Invalid provider configuration was supplied. Provider operations likely to fail", "error", err)
		return nil, nil
	}

	return cfg, nil
}
*/

type Client struct {
	dc dynamic.Interface
	//discoveryClient discovery.CachedDiscoveryInterface
	mapper meta.RESTMapper
}

// getMapping returns the RESTMapping for the provided resource.
func (r *Client) getMapping(obj *unstructured.Unstructured) (*meta.RESTMapping, error) {
	return r.mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
}

func (r *Client) Get(ctx context.Context, obj *unstructured.Unstructured, options metav1.GetOptions) (*unstructured.Unstructured, error) {
	m, err := r.getMapping(obj)
	if err != nil {
		return nil, err
	}

	var newObj *unstructured.Unstructured
	if m.Scope == meta.RESTScopeNamespace {
		newObj, err = r.dc.Resource(m.Resource).Namespace(obj.GetNamespace()).Get(ctx, obj.GetName(), options)
	} else {
		newObj, err = r.dc.Resource(m.Resource).Get(ctx, obj.GetName(), options)
	}
	if err != nil {
		return nil, err
	}
	return newObj, nil
}

func (r *Client) Create(ctx context.Context, obj *unstructured.Unstructured, options metav1.CreateOptions) (*unstructured.Unstructured, error) {
	m, err := r.getMapping(obj)
	if err != nil {
		return nil, err
	}
	var newObj *unstructured.Unstructured
	if m.Scope == meta.RESTScopeNamespace {
		newObj, err = r.dc.Resource(m.Resource).Namespace(obj.GetNamespace()).Create(ctx, obj, options)
	} else {
		newObj, err = r.dc.Resource(m.Resource).Create(ctx, obj, options)
	}
	if err != nil {
		return nil, err
	}
	return newObj, nil
}

func (r *Client) Update(ctx context.Context, obj *unstructured.Unstructured, options metav1.UpdateOptions) (*unstructured.Unstructured, error) {
	m, err := r.getMapping(obj)
	if err != nil {
		return nil, err
	}
	var newObj *unstructured.Unstructured
	if m.Scope == meta.RESTScopeNamespace {
		newObj, err = r.dc.Resource(m.Resource).Namespace(obj.GetNamespace()).Update(ctx, obj, options)
	} else {
		newObj, err = r.dc.Resource(m.Resource).Update(ctx, obj, options)
	}
	if err != nil {
		return nil, err
	}
	return newObj, nil
}

func (r *Client) Delete(ctx context.Context, obj *unstructured.Unstructured, options metav1.DeleteOptions) error {
	m, err := r.getMapping(obj)
	if err != nil {
		return err
	}
	if m.Scope == meta.RESTScopeNamespace {
		err = r.dc.Resource(m.Resource).Namespace(obj.GetNamespace()).Delete(ctx, obj.GetName(), options)
	} else {
		err = r.dc.Resource(m.Resource).Delete(ctx, obj.GetName(), options)
	}
	if err != nil {
		return err
	}
	return nil
}

/*
// overlyCautiousIllegalFileCharacters matches characters that *might* not be supported.  Windows is really restrictive, so this is really restrictive
var overlyCautiousIllegalFileCharacters = regexp.MustCompile(`[^(\w/.)]`)

// computeDiscoverCacheDir takes the parentDir and the host and comes up with a "usually non-colliding" name.
func computeDiscoverCacheDir(parentDir, host string) string {
	// strip the optional scheme from host if its there:
	schemelessHost := strings.Replace(strings.Replace(host, "https://", "", 1), "http://", "", 1)
	// now do a simple collapse of non-AZ09 characters.  Collisions are possible but unlikely.  Even if we do collide the problem is short lived
	safeHost := overlyCautiousIllegalFileCharacters.ReplaceAllString(schemelessHost, "_")
	return filepath.Join(parentDir, safeHost)
}
*/
