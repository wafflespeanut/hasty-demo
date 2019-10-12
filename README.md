## Hasty demo

[![Build Status](https://api.travis-ci.org/wafflespeanut/hasty-demo.svg?branch=master)](https://travis-ci.org/wafflespeanut/hasty-demo)

A hobby project for designing a simple scalable image service. Backend technologies include:

- Go (1.12): I could've picked Python, but it requires a runtime, whereas a compiled (statically linked) Go executable is ~5-10 MB and the docker image is very thin. I could've gone for Rust - it's much faster than Go, the executables are *slightly* bigger, and has some popular frameworks ([tide](https://github.com/rustasync/tide/) or [actix-web](https://github.com/actix/actix-web)), but I've spent an awful lot of time with Rust over the last few years, so I wanted to Go.
- CockroachDB:

### Service

The API itself has four endpoints:

 - `POST /link`:
 - `POST /uploads/{temp-id}`:
 - `GET /images/{id}`:
 - `GET /stats`:
