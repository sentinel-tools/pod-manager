# Pod Manager

The pod-manager tool is intended to provide a CLI Tool to handle some of the
administrivia of managing a Redis Pod in Sentinel.

For example, normally to remove the pod 'pod1' from your bank of sentinels, you
would have to iterate over each sentinel, and issue a 'sentinel remove pod1'
command to each one.

With pod-manager you run `pod-manager -podname pod1 -remove'.

There are several other commands such as 'validatesentinels', 'failover', and 'reset'.
