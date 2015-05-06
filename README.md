[![Build Status](https://travis-ci.org/sentinel-tools/pod-manager.svg?branch=master)](https://travis-ci.org/sentinel-tools/pod-manager)
# Pod Manager

The pod-manager tool is intended to provide a CLI Tool to handle some of the
administrivia of managing a Redis Pod in Sentinel.

For example, normally to remove the pod 'p2' from your bank of sentinels, you
would have to iterate over each sentinel, and issue a 'sentinel remove p2'
command to each one.

With pod-manager you run `pod-manager pod remove p2'.

There are several other sub-commands such as 'validatesentinels', 'failover',
and 'reset'.

# Operational Requirements
When built with sentinel config file support, pod-manager requires read access
to said config file. It will also need connectivity to the Sentinels in the
cluster (it will build the list from the config file). Finally, for
node-specific actions it will need access to the Redis instances in the pod.


# Example Usage

Displaying information about the Pod 'p2':
```shell
# pod-manager pod info p2
Podname: p2
========================
Master: 127.0.0.1:6400
Quorum: 2
Auth Token: foo
Known Sentinels: 
        127.0.0.1:26380 
        127.0.0.1:26381 
        127.0.0.1:26379 
Known Slaves: 
        127.0.0.1:6401

Settings: 
cli string: redis-cli -h 127.0.0.1 -p 6400 -a foo

```

The same, but with JSON instead:
```shell
# pod-manager pod info -j p2
{"Name":"p2","MasterIP":"127.0.0.1","MasterPort":"6400","Authpass":"foo","KnownSentinels":["127.0.0.1:26380","127.0.0.1:26381","127.0.0.1:26379"],"KnownSlaves":["127.0.0.1:6401"],"Settings":null,"Quorum":"2","BadDirectives":null}
```


Removing a pod from the sentinel cluster:
```shell
pod-manager pod remove pod1
```


##Validating Authentication
By passing the '-authcheck' flag pod-manager will attempt to conenct to
the master and every KnownSlave and issue a ping, authenticating with
the auth-pass entry. If any instances fail the check results of all
checks will be displayed.

#Sentinel Validation 
By using the `validatesentinels` subcommand you can have pod-manager connect to
each sentinel in the config file and request the master info from them. If any
of the KnownSentinels do not have the pod in their list (ie. returning null or
some other error) or are unreachable, pod-manager will tell you how many
sentinels were validated vs how many were expected.

This is useful for checking the number of sentinels Sentinel must have a
majority of in order to elect a leader in the event of an `+odown` event on a
master.


# Which Pod Tool?
Why pod-connector and pod-manager? Pod Connector's primary purpose is to
provide connectivity to the specified pod, with a bit of info available. Pod
Manager, however, is designed to make changes to sentinel and in some cases
directly to the instances in the pod.

You might give access to pod-connector people whom you are willing to allow
connectivity to the pod, but reserve pod-manager access for those who manage
sentinel and the pod. Different needs, different tools. This is much simpler
than yet another user system in a tool.


# Bash Completion
Add the pod-manager_completion file to your bash completions directory (or
source it directly) to add bash completion for pod-manager.
