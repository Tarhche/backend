# Website

this is the source code of our website

## How to run

You can run the whole project localy using `docker compose up --detach`

Or you can use our Makefile commands

```
make up
```

## Makefile Guide

- to see a container's logs, the `make logs <container_name>` command can be used. the `<container_name>` refers to the name of your target container.

```sh
make logs-app
```

- to attach to `sh` in a container `make sh <container_name>` can be used. the `<container_name>` refers to the name of your target container.

```sh
make sh-app
```

## production grade builds

We build and push the images to our [github packages](https://github.com/orgs/Tarhche/packages).
using these images you can deploy your website.
