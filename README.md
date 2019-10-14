## Hasty demo

[![Build Status](https://api.travis-ci.org/wafflespeanut/hasty-demo.svg?branch=master)](https://travis-ci.org/wafflespeanut/hasty-demo)

A hobby project for designing a simple scalable image service. My reasoning for choice of technologies:

- Go (1.12): I could've picked Python, but it requires a runtime, whereas a compiled (statically linked) Go executable is ~5-10 MB and the docker image is very thin. I could've gone for Rust - it's much faster than Go, the executables are *slightly* bigger, and has some popular frameworks ([tide](https://github.com/rustasync/tide/) or [actix-web](https://github.com/actix/actix-web)), but I've spent an awful lot of time with Rust over the last few years, so I wanted to Go.
- [CockroachDB](https://www.cockroachlabs.com/): CockroachDB is a distributed PostgreSQL-compatible database. I usually prefer PostgreSQL over MySQL or SQLite for applications that may involve complexity in the future.

### Development

 - `make build` to build the app.
 - `make image` to build the docker image.
 - `make run` to build the image and spawn a container along with a container of CockroachDB.
 - `make test` to run tests.

### Service endpoints

The API has the following endpoints:

Endpoint | Auth | Description
-------- | ---- | -----------
`POST /admin/ephemeral-links` | Yes | <p>Accepts an expiry datetime or duration in ISO 8601 format and generates an ephemeral link.</p> <pre><p><code>curl -H "X-Access-Token: foobar" -d '{"sinceNow": "PT1H"}' http://localhost:3000/admin/ephemeral-links</code></p><p><code>{"relativePath": "/uploads/booya", "expiresOn": "2019-10-14T06:21:46Z"}</code></p></pre>
`GET  /admin/stats` | Yes | <p>Retrieves some statistics about the service.</p> <pre><p><code>curl -H "X-Access-Token: foobar" http://localhost:3000/admin/stats</code></p><p><code>{"popularFormat": {"format": "JPEG", "uploads": 12}, "top10CameraModels": [{"model": "unknown", "uploads": 17}, {"model": "\\"iPhone 8 Plus\\"", "uploads": 7}], "uploadFrequency30Days": [{"date": "2019-10-14T00:00:00Z", "uploads": 7}, {"date": "2019-10-13T00:00:00Z", "uploads": 10}, {"date": "2019-10-10T00:00:00Z", "uploads": 7}]}</code></p></pre>
`POST /{ephemeral-link}` | No | <p>Accepts one or more images in `multipart/form-data`. Each part contains an image and must have `Content-Type` header set to `image/*`.</p> <pre><p><code>curl -F "image=@$HOME/sample.png;type=image/png" -F "image=@$HOME/sample.jpg;type=image/jpeg" http://localhost:3000/uploads/booya</code></p><p><code>{"processed": [{"name": "sample.png", "id": "EvdsbIHYealgSWpuhggiRHvwfZVJPdFDHAiWjzoWmPMhTMKO", "hash": "5516da0a747f6b7b043cc4c8349815dcf462d748a2f5d1fa35c06637bef075ef", "size": 22894}, {"name": "sample.jpg", "id": "nUGmxeKbJithbDQmiCDgpaYnhUqfRGkNKzdNztuhhOxTBCvN", "hash": "b21f6bfd6e910d0214f2117cec06e2e01a5f7e47e7ef2b349a6de306edf2e9fc", "size": 524499}]}</code></p></pre><p>**NOTE:** This endpoint computes the SHA-256 hash of each image against known image hashes to filter duplicates. Should a duplicate be found, the ID of that image is returned and the uploaded image is discarded.</p>
`GET  /images/{id}` | No | <p>Streams an image if it exists for the given ID.</p> <pre><code>wget -O image http://localhost:3000/images/someImageId</code></pre>

### Design

I've followed service-oriented design and repository pattern (with some modifications) for processing the requests. `ImageService` takes care of validation and communicating with `ImageRepository` to offer a response. It doesn't know anything about HTTP (the handlers are isolated elsewhere). The repository acts as a bridge between the service and the store, and also offers some caching (using an LRUCache) for quickly responding to hot paths. It also aids testing.

We need to access the repository cache from different goroutines. Instead of locking the entire repository, we use channels within the repository and expose command-like methods to the service layer. There are 3 goroutines for persisting in and querying the repository - one for data, one for streaming images, and another for processing stored images (right now, we extract metadata in that process).

If the repository doesn't have something in the cache, it talks to the store to get it. Repository cannot cache everything, so a few calls need the store. We have two store interfaces - `DataStore` for API calls and `ObjectStore` for streaming and processing objects. This abstraction helps with isolating the logic from driver-specific code. Right now, we have `PostgreSQLStore` which implements `DataStore` for using PostgreSQL-compatible database in the backend, and `FileStore` which implements `ObjectStore` for storing and retrieving objects.

### Real-time vs batch processing pipeline

Batch processing is typically for compute-intensive tasks. We already use batch processing for analyzing stored images (right now, in the same application, but won't be the case as we scale).

As we stream images, we can obtian the basic metadata (size, hash, etc.) and store it in database, but we can't get all metadata (format, dimensions, camera model, etc.), because existing third-party libraries consume either a reader or a buffer - we can't offer them the former as we have to stream it to storage and we can't do the latter, as that would mean our memory can quickly increase for smaller servers when multiple users upload images.

We *could* grab the metadata on the fly, but still, that involves writing our own parser and that parser should support all the formats we're planning to support, and it adds further computing time. What we can do instead is queueing images for batch processing based on size. We keep feeding bytes to our parser, and if it's within the size, then we can store the metadata straightaway, but if the size exceeds a threshold, we can drop the parser and queue it for processing later. As we scale, this processing will be done by separate containers.

One other use for batch processing is cleanup and maintenance. If we find that an image is not useful or (after some interval) no longer useful, then we need to archive it (move it to cold storage or something) or get rid of it entirely (which is the case for big files that aren't images or are corrupted).

### Scaling

Technologies that could be used:

- Orchestration (e.g., Kubernetes): Since docker containers are a requirement, we need an orchestrator to scale and monitor the containers when needed.
- Load balancer (depends on IaaS provider): As the service containers scale, we need a load balancer to route requests to the containers appropriately. We may not need a reverse proxy, because orchestrators like Kubernetes offer ingress/egress controllers out of the box.
- Object storage or some CDN (e.g., AWS S3): As the services scale, we need to be able to proxy and upload user images directly to some object storage service, because there's only so much a file system can take.
- Message Queue (e.g., Kafka, NATS): Distributed message queues can be used for passing events across the system.
  - For instance, once the service streams the images to storage, it can send the image information to the message queue, and some other service can pick it up for processing it in batches.
  - Database write calls can also be passed through MQ, because we need to be able to ensure that writes always happen in the order they were queued, and we need the guarantee that none of the writes have failed. Should some failure occur, we need to retry it again after some time.
- Distributed database (e.g., CockroachDB): Similar to other services, the database should also support scaling and distribution to mitigate data corruption and disasters.

Despite these and perhaps one of the most important layer: **logging**. We need to collect logs in some centralized service for future debugging.

### Questions

1. What's the extent of image formats we should support? Exif data (camera model, geotagging, etc.) is available only for popular formats such as JPEG (and maybe PNG), wheras some (say, GIF or BMP) don't have that data, so are they still useful?
2. Should we store the images exactly how we received them? Can they be compressed? (say, HEIC format)
3. Does compliance come into play? For example, certain businesses may not want their data (in our case, images) to be moved out of certain regions. IaaS services need to be picked based on that.
4. A few more businesses may demand on-premise installation (bare metal). Should we support that? If we did, then we won't get any of the image data for our processing (say, training AI models or collecting stats), and the design need to be rethought.
5. Are we going to be cloud-agnostic?

We won't have to worry about AI (or other) tooling because any kind of language/technology can be picked in service-oriented design. If we don't have proper tooling for some feature in say, Go, and we do have it in say, Python, then all we need to do is to create a bridge for the Go application to talk to the Python application when we process the images for that feature (regardless of whether it's real-time or batch).
