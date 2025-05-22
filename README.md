# tiny-http

**tiny-http** is a (very) simple HTTP server written in Go.

## Current features/Limitations

- Only supports GET method
- Can serve the contents of a directory
- Supports response Gzip compression
- **No tests**

## How to build

To build run:

```shell
go build -o tiny-http ./cmd
```

Optionally, you can build it in a Docker image:

```shell
docker build -t tiny-server:latest .
```

## How to run

To learn how to start it run:

```shell
./tiny-http
```
Or, if you want to use Docker (don't forget build the image first):

```shell
docker run -it --rm \
    -p 8080:8080 \
    -v <directory-to-server>:/<directory-inside-container> \
    tiny-http:latest ./tiny-http --directory <directory-inside-container>
```

---
Happy Hacking!
