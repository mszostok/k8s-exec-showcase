package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/spf13/pflag"
	"golang.org/x/crypto/ssh/terminal"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

var (
	containerName = pflag.StringP("container", "c", "", "Container in which to execute the command. Defaults to only container if there is only one container in the pod.")
	namespace     = pflag.StringP("namespace", "n", "", "Namespace where pod is deployed. Defaults to default.")
	command       = pflag.StringSliceP("command", "e", []string{"sh"}, "The remote command to execute. Defaults to sh.")
	help          = pflag.BoolP("help", "h", false, "Print help for commands.")
)

func main() {
	validUsageAndExitOnFailure()

	kubeconfig := getConfig(os.Getenv("KUBECONFIG"))

	k8sCliCfg, err := kubeconfig.ClientConfig()
	fatalOnErr(err, "while getting client cfg")

	k8sCoreCli, err := corev1.NewForConfig(k8sCliCfg)
	fatalOnErr(err, "while creating core client")

	podName := pflag.Arg(0)
	ns, err := determineNamespace(kubeconfig)
	fatalOnErr(err, "while getting default namespace")

	req := k8sCoreCli.RESTClient().
		Post().
		Namespace(ns).
		Resource("pods").
		Name(podName).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: *containerName,
			Command:   *command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	fmt.Printf("Exec to POD %s/%s with command %q\n", ns, podName, *command)
	exec, err := remotecommand.NewSPDYExecutor(k8sCliCfg, http.MethodPost, req.URL())
	fatalOnErr(err, "while creating SPDY executor")

	// By default terminal starts in cooked mode (canonical).
	// In this mode, keyboard input is preprocessed before being given to a program.
	// In Raw mode the data is passed to the program without interpreting any of the special characters, by that
	// we are turning off the ECHO feature because we already connecting the streams between our terminal and the remote shell process
	// Stdin: os.Stdin -> Stdout: os.Stdout,
	oldState, err := terminal.MakeRaw(0)
	fatalOnErr(err, "while putting terminal into raw mode")
	defer terminal.Restore(0, oldState)

	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    true,
	})
	fatalOnErr(err, "connect to process")
}

func determineNamespace(cfg clientcmd.ClientConfig) (string, error) {
	if *namespace != "" {
		return *namespace, nil
	}
	ns, _, err := cfg.Namespace()
	return ns, err
}

func getConfig(explicitKubeconfig string) clientcmd.ClientConfig {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	rules.ExplicitPath = explicitKubeconfig

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
}

func fatalOnErr(err error, ctx string) {
	if err != nil {
		log.Fatalf("%s: %v", ctx, err)
	}
}

func validUsageAndExitOnFailure() {
	pflag.Parse()

	if *help {
		printHelpAndExit()
	}
	if pflag.NArg() == 0 || pflag.NArg() > 1 {
		printArgErrMsgAndExit()
	}
}

func printHelpAndExit() {
	fmt.Println("Execute a command in a container.")
	fmt.Printf("Usage: \n \t '%s [-c CONTAINER] [-n NAMESPACE] POD_NAME [COMMAND]'\n", os.Args[0])
	fmt.Println("Options:")
	pflag.PrintDefaults()
	os.Exit(0)
}

func printArgErrMsgAndExit() {
	fmt.Printf("Expected '%s [-c CONTAINER] [-n NAMESPACE] POD_NAME [COMMAND]'\n", os.Args[0])
	fmt.Printf("POD is a required argument for the %s command\n", os.Args[0])

	fmt.Println()
	fmt.Println("Options:")
	pflag.PrintDefaults()
	os.Exit(1)
}
