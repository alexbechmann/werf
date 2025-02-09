package helm

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	helm_kube "helm.sh/helm/v3/pkg/kube"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/werf/client"
	"helm.sh/helm/v3/pkg/werf/kubeclient"
	"helm.sh/helm/v3/pkg/werf/mutator"
	"helm.sh/helm/v3/pkg/werf/resourcetracker"
	"helm.sh/helm/v3/pkg/werf/resourcewaiter"
	"sigs.k8s.io/yaml"

	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/logboek"
	"github.com/werf/werf/pkg/util"
)

const FEATURE_TOGGLE_ENV_EXPERIMENTAL_DEPLOY_ENGINE = "WERF_EXPERIMENTAL_DEPLOY_ENGINE"

type InitActionConfigOptions struct {
	StatusProgressPeriod      time.Duration
	HooksStatusProgressPeriod time.Duration
	KubeConfigOptions         kube.KubeConfigOptions
	ReleasesHistoryMax        int
	RegistryClient            *registry.Client
}

func InitActionConfig(ctx context.Context, kubeInitializer KubeInitializer, releaseName, namespace string, envSettings *cli.EnvSettings, actionConfig *action.Configuration, opts InitActionConfigOptions, extraMutators []mutator.RuntimeResourceMutator) error {
	configGetter, err := kube.NewKubeConfigGetter(kube.KubeConfigGetterOptions{
		KubeConfigOptions: opts.KubeConfigOptions,
		Namespace:         namespace,
	})
	if err != nil {
		return fmt.Errorf("error creating kube config getter: %w", err)
	}

	*envSettings.GetConfigP() = configGetter
	*envSettings.GetNamespaceP() = namespace
	if opts.KubeConfigOptions.Context != "" {
		envSettings.KubeContext = opts.KubeConfigOptions.Context
	}
	if opts.KubeConfigOptions.ConfigPath != "" {
		envSettings.KubeConfig = opts.KubeConfigOptions.ConfigPath
	}
	if opts.ReleasesHistoryMax != 0 {
		envSettings.MaxHistory = opts.ReleasesHistoryMax
	}

	helmDriver := os.Getenv("HELM_DRIVER")
	if err := actionConfig.Init(envSettings.RESTClientGetter(), envSettings.Namespace(), helmDriver, logboek.Context(ctx).Debug().LogF); err != nil {
		return fmt.Errorf("action config init failed: %w", err)
	}
	if helmDriver == "memory" {
		loadReleasesInMemory(envSettings, actionConfig)
	}

	kubeClient := actionConfig.KubeClient.(*helm_kube.Client)
	kubeClient.Namespace = namespace
	kubeClient.ResourcesWaiter = NewResourcesWaiter(kubeInitializer, kubeClient, time.Now(), opts.StatusProgressPeriod, opts.HooksStatusProgressPeriod)
	kubeClient.Extender = NewHelmKubeClientExtender()

	actionConfig.Log = func(f string, a ...interface{}) {
		logboek.Context(ctx).Info().LogFDetails(fmt.Sprintf("%s\n", f), a...)
	}

	if opts.RegistryClient != nil {
		actionConfig.RegistryClient = opts.RegistryClient
	}

	if util.GetBoolEnvironmentDefaultFalse(FEATURE_TOGGLE_ENV_EXPERIMENTAL_DEPLOY_ENGINE) {
		deferredKubeClient := kubeclient.NewDeferredKubeClient(configGetter)

		waiter := resourcewaiter.NewResourceWaiter(deferredKubeClient.Dynamic(), deferredKubeClient.Mapper())

		tracker := resourcetracker.NewResourceTracker(opts.StatusProgressPeriod, opts.HooksStatusProgressPeriod)

		cli, err := client.NewClient(deferredKubeClient.Static(), deferredKubeClient.Dynamic(), deferredKubeClient.Discovery(), deferredKubeClient.Mapper(), waiter)
		if err != nil {
			return fmt.Errorf("error creating client: %w", err)
		}
		cli.AddTargetResourceMutators(extraMutators...)
		cli.AddTargetResourceMutators(
			mutator.NewReplicasOnCreationMutator(),
			mutator.NewReleaseMetadataMutator(releaseName, namespace),
		)

		actionConfig.DeferredKubeClient = deferredKubeClient
		actionConfig.Waiter = waiter
		actionConfig.Tracker = tracker
		actionConfig.Client = cli
	}

	return nil
}

// This function loads releases into the memory storage if the
// environment variable is properly set.
func loadReleasesInMemory(envSettings *cli.EnvSettings, actionConfig *action.Configuration) {
	filePaths := strings.Split(os.Getenv("HELM_MEMORY_DRIVER_DATA"), ":")
	if len(filePaths) == 0 {
		return
	}

	store := actionConfig.Releases
	mem, ok := store.Driver.(*driver.Memory)
	if !ok {
		// For an unexpected reason we are not dealing with the memory storage driver.
		return
	}

	actionConfig.KubeClient = &kubefake.PrintingKubeClient{Out: ioutil.Discard}

	for _, path := range filePaths {
		b, err := ioutil.ReadFile(path)
		if err != nil {
			log.Fatal("Unable to read memory driver data", err)
		}

		releases := []*release.Release{}
		if err := yaml.Unmarshal(b, &releases); err != nil {
			log.Fatal("Unable to unmarshal memory driver data: ", err)
		}

		for _, rel := range releases {
			if err := store.Create(rel); err != nil {
				log.Fatal(err)
			}
		}
	}
	// Must reset namespace to the proper one
	mem.SetNamespace(envSettings.Namespace())
}
