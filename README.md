# Skripture

> So shall my config be that goeth forth out of my Kubernetes cluster: it shall not deliver unto me void, but it shall accomplish that which I please, and it shall prosper in the environment whereto I sent it.
>
> -- Isaiah 55:11 (more or less)

## What is Skripture?

**Put simply:** Skripture builds you a shell environment which matches a pod running in Kubernetes.

[Skripture][0] retrieves all environment variables from a selected container in a kubernetes cluster, then injects it into the environment of a subshell. [Skripture][0] was inspired by [aws-vault][1], which does a similar thing for injecting ephemeral AWS keys into a shell for you to use.

## Why would I do this?

The most basic way to develop with a Kubernetes cluster locally is to make a change, build a docker image, push it to a registry, then instruct Kubernetes to pull the new image and run it. This works well, but it's incredibly slow - and this is especially true if you are working on a large project with a complex release procedure. Debugging, accessing a REPL, or retrieving artifacts from the container is often annoying and difficult, and all of your tools will require re-configuration to account for the project being inside Kubernetes.

It'd be much better to run the application you are developing **outside** the Kubernetes cluster, but to be able to communicate with Kubernetes service hostnames as if you were **inside** the cluster (I use [localizer][2] for this purpose), and to inherit all the normal configuration (that's where [Skripture][0] comes in). With [Skripture][0], you can say goodbye to out of date development configuration file hanging around unstaged in your repo, and instead leave it to kubernetes to provide you with the environment you need!

## How do I use this?

```shell
$ skripture --help
Usage of ./bin/skripture:
  -kubeconfig string
    	(optional) absolute path to the kubeconfig file (default "/home/john/.kube/config")
  -namespace string
    	Namespace to search within (default "default")
  -pod-selector string
    	Pod Selector

$ skripture --namespace="production" --pod-selector="app=v2-production"
2021/08/27 01:48:34 [v2-production] Searching for foreign configuration..
2021/08/27 01:48:34 [v2-production] Searching for local environment...
2021/08/27 01:48:34 Opening shell with environment: APNS_CERT, DATABASE_URL, LIVEVIEW_SIGNING_SALT, APNS_KEY, SECRET_KEY_BASE, FCM_TOKEN, RELEASE_COOKIE, USER_SALT, REDIS_HOST, S3_BUCKET_REGION, REDIS_PORT, S3_BUCKET, STACK_NAME, APNS_ENVIRONMENT, HOST, PORT, REDIS_POOL_SIZE
2021/08/27 01:48:34 Exec command /bin/zsh -i -s
2021/08/27 01:48:34 Found executable /bin/zsh
$ env | grep PORT
REDIS_PORT=6379
PORT=4000
```

## TODO

- [ ] Refactor & Write unit tests
- [ ] Use Viper & a better logger for a nicer CLI experience
- [ ] Allow the user to select a container instead of merging the environment of all containers that match a pods selector
- [ ] Provide support for binary as well as text configuration
- [ ] Provide file-mounting support for configuration files which need to be on disk

[0]: https://github.com/johnhamelink/skripture
[1]: https://github.com/99designs/aws-vault
[2]: https://github.com/getoutreach/localizer
