![Benthos](icon.png "Benthos")

[![godoc for redpanda-data/benthos][godoc-badge]][godoc-url]
[![Build Status][actions-badge]][actions-url]

> Note: if you are looking for the original benthos repo with connectors, it moved here: https://github.com/redpanda-data/connect


Benthos is a framework for creating declarative stream processors where a pipeline of one or more sources, an arbitrary series of processing stages, and one or more sinks can be configured in a single config:

```yaml
input:
  gcp_pubsub:
    project: foo
    subscription: bar

pipeline:
  processors:
    - mapping: |
        root.message = this
        root.meta.link_count = this.links.length()
        root.user.age = this.user.age.number()

output:
  redis_streams:
    url: tcp://TODO:6379
    stream: baz
    max_in_flight: 20
```

### Delivery Guarantees

Delivery guarantees [can be a dodgy subject](https://youtu.be/QmpBOCvY8mY). Benthos processes and acknowledges messages using an in-process transaction model with no need for any disk persisted state, so when connecting to at-least-once sources and sinks it's able to guarantee at-least-once delivery even in the event of crashes, disk corruption, or other unexpected server faults.

This behaviour is the default and free of caveats, which also makes deploying and scaling Benthos much simpler.

## Lint

Benthos uses [golangci-lint][golangci-lint] for linting, which you can install with:

```shell
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

And then run it with `make lint`.

[godoc-badge]: https://pkg.go.dev/badge/github.com/redpanda-data/benthos/v4/public
[godoc-url]: https://pkg.go.dev/github.com/redpanda-data/benthos/v4/public
[actions-badge]: https://github.com/redpanda-data/benthos/actions/workflows/test.yml/badge.svg
[actions-url]: https://github.com/redpanda-data/benthos/actions/workflows/test.yml

[golangci-lint]: https://golangci-lint.run/
