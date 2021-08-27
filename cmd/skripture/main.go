package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	osexec "golang.org/x/sys/execabs"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type ctx struct {
	Clientset  *kubernetes.Clientset
	Containers []v1.Container
	Namespace  string
	Env        envKV
}

type envKV struct {
	Keys []string
	Data map[string]string
}

func (e *envKV) set(key string, value string) {
	e.Keys = append(e.Keys, key)
	e.Data[key] = value
}

func (e envKV) setFromHost(key string) {
	e.set(key, os.Getenv(key))
}

// TODO: I don't like the name of this
func (e envKV) toStringList() []string {
	env := []string{}
	for key, value := range e.Data {
		env = append(env, key+"="+value)
	}
	return env
}

func (e *envKV) initialize() {
	e.Keys = []string{}
	e.Data = map[string]string{}

	// Set some default variables which are important for the terminal prompt.
	// Config which is set will be overridden.
	e.setFromHost("TERM")
	e.setFromHost("LANG")
}

func main() {
	var kubeconfig *string
	var namespace *string
	var podSelector *string

	// TODO: Use Viper instead, it's much nicer
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	podSelector = flag.String("pod-selector", "", "Pod Selector")
	namespace = flag.String("namespace", "default", "Namespace to search within")
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	ctx := ctx{
		Clientset:  clientset,
		Namespace:  *namespace,
		Containers: []v1.Container{},
	}

	pods := ctx.getPods(*podSelector)

	if len(pods) == 0 {
		log.Fatalln("Could not find any pods which matched the selector '" + *podSelector + "' in '" + *namespace + "'")
		os.Exit(1)
	}

	ctx.Env.initialize()

	for _, pod := range pods {
		ctx.getContainers(&pod)
		for _, container := range ctx.Containers {
			ctx.getExternalConfig(container)
			ctx.getLocalEnv(container)
		}
	}

	openShell([]string{}, ctx.Env)
}

func openShell(args []string, env envKV) error {
	log.Println("Opening shell with environment: " + strings.Join(env.Keys, ", "))
	args = append(args, "-i")
	args = append(args, "-s")
	if !supportsExecSyscall() {
		return execCmd(os.Getenv("SHELL"), args, env.toStringList())
	}
	return execSyscall(os.Getenv("SHELL"), args, env.toStringList())
}

func execCmd(command string, args []string, env []string) error {
	log.Printf("Starting child process: %s %s", command, strings.Join(args, " "))

	cmd := osexec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		for {
			sig := <-sigChan
			_ = cmd.Process.Signal(sig)
		}
	}()

	if err := cmd.Wait(); err != nil {
		_ = cmd.Process.Signal(os.Kill)
		return fmt.Errorf("failed to wait for command termination: %v", err)
	}

	waitStatus := cmd.ProcessState.Sys().(syscall.WaitStatus)
	os.Exit(waitStatus.ExitStatus())
	return nil
}

func supportsExecSyscall() bool {
	return runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd"
}

func execSyscall(command string, args []string, env []string) error {
	log.Printf("Exec command %s %s", command, strings.Join(args, " "))

	argv0, err := osexec.LookPath(command)
	if err != nil {
		return fmt.Errorf("couldn't find the executable '%s': %w", command, err)
	}

	log.Printf("Found executable %s", argv0)

	argv := make([]string, 0, 1+len(args))
	argv = append(argv, command)
	argv = append(argv, args...)

	return syscall.Exec(argv0, argv, env)
}

func (c ctx) getPods(labelSelector string) []v1.Pod {
	pods, err := c.Clientset.CoreV1().Pods(c.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: labelSelector})

	if err != nil {
		return []v1.Pod{}
	}

	return pods.Items
}

func (c *ctx) getContainers(pod *v1.Pod) {
	c.Containers = append(
		c.Containers,
		pod.Spec.Containers...,
	)
}

func (c *ctx) getLocalEnv(container v1.Container) {
	log.Println("[" + container.Name + "] Searching for local environment...")
	for _, e := range container.Env {
		c.Env.set(e.Name, e.Value)
	}
}

func (c *ctx) getExternalConfig(container v1.Container) {
	log.Println("[" + container.Name + "] Searching for foreign configuration..")

	// TODO: This is wrong: we should pick a single container
	//       from the list, or else choose a random one.
	for idx := range container.EnvFrom {
		foreignEnv := &container.EnvFrom[idx]

		if foreignEnv.ConfigMapRef != nil {
			for key, value := range c.getConfigMap(*foreignEnv.ConfigMapRef) {
				c.Env.set(key, value)
			}
		}

		if foreignEnv.SecretRef != nil {
			for key, value := range c.getSecret(*foreignEnv.SecretRef) {
				c.Env.set(key, value)
			}
		}
	}
}

func (c ctx) getConfigMap(cm v1.ConfigMapEnvSource) map[string]string {
	//log.Printf("Found ConfigMap: %s", cm.Name)
	configMap, err := c.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(context.TODO(), cm.Name, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	return configMap.Data
}

func (c ctx) getSecret(s v1.SecretEnvSource) map[string]string {
	//log.Printf("Found Secret: %s", s.Name)
	secret, err := c.Clientset.CoreV1().Secrets(c.Namespace).Get(context.TODO(), s.Name, metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	env := map[string]string{}
	for key, byteValue := range secret.Data {
		// TODO: Don't assume this is always a string
		env[key] = string(byteValue)
	}

	return env
}
