package apm

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/newrelic/k8s-agents-operator/api/current"
)

var ErrInjectorAlreadyRegistered = errors.New("injector already registered in registry")

// ContainerInjector is used to inject a specific container, rather than always the first container
type ContainerInjector interface {
	Language() string
	Accepts(inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod) bool
	ConfigureClient(client client.Client)
	InjectContainer(ctx context.Context, inst current.Instrumentation, ns corev1.Namespace, pod corev1.Pod, containerName string) (corev1.Pod, error)
}

type Injectors []ContainerInjector

func (i Injectors) Names() []string {
	injectorNames := make([]string, len(i))
	for j, injector := range i {
		injectorNames[j] = injector.Language()
	}
	return injectorNames
}

type InjectorRegistery struct {
	injectors   []ContainerInjector
	injectorMap map[string]struct{}
	mu          *sync.Mutex
}

func NewInjectorRegistry() *InjectorRegistery {
	return &InjectorRegistery{
		injectorMap: make(map[string]struct{}),
		mu:          &sync.Mutex{},
	}
}

func (ir *InjectorRegistery) Register(injector ContainerInjector) error {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	if _, ok := ir.injectorMap[injector.Language()]; ok {
		return ErrInjectorAlreadyRegistered
	}
	ir.injectors = append(ir.injectors, injector)
	return nil
}

func (ir *InjectorRegistery) MustRegister(injector ContainerInjector) {
	err := ir.Register(injector)
	if err != nil {
		panic(err)
	}
}

func (ir *InjectorRegistery) GetInjectors() Injectors {
	ir.mu.Lock()
	defer ir.mu.Unlock()
	injectors := make([]ContainerInjector, len(ir.injectors))
	copy(injectors, ir.injectors)
	sort.Slice(injectors, func(i, j int) bool {
		return strings.Compare(injectors[i].Language(), injectors[j].Language()) < 0
	})
	return injectors
}

var DefaultInjectorRegistry = NewInjectorRegistry()
