# go-do-snapshot
Take a Digital Ocean snapshot and transfer it to another region

## Usage

Define an environment variable `DO_TOKEN` with a valid Digital Ocean Token

go-do-snapshot is meant to be run on the droplet you intend to snapshot. It discovers the droplet ID using metadata.


take a snapshot

```
./go-do-snapshot
```

Set a destination to transfer to:

```
./go-do-snapshot -d SFO3
```

Set multiple destinations:

```
./go-do-snapshot -d SFO3 -d NYC3
```

Automatically clean up old snapshots by defining how many to keep:

```
./go-do-snapshot -d SFO3 -k 4
```

Delete all snapshots by setting k to 0. No snapshot will be taken. 

```
./go-do-snapshot -k 0
```

## Crontab
```
# weekly snapshot on Sunday at 1am with transfer to NYC3, keeping 3 snapshots
0 1 * * 0 /usr/local/bin/go-do-snapshot -d NYC3 -k 3 2>&1 | logger -t "go-do-snapshot"
```