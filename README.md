# Husky Service

``Husky is handling everything related to user notifications.``

### Development

These are the crucial/critical operations you will need when developing `Husky`:

1. If you need a DID token for `magic.link` for testing locally, see https://github.com/magiclabs/magic-admin-go/blob/master/token/did_test.go
2. `make run-husky`
    1. This runs the actual read service.
    2. It will feed off of the properties in `./application.yaml`
    3. By default, https://localhost/notifications/r runs the Open API (Swagger) entrypoint.
3. `make run-husky-pack`
   1. This runs the actual write service.
   2. It will feed off of the properties in `./application.yaml`
   3. By default, https://localhost:1443/notifications/w runs the Open API (Swagger) entrypoint.
4. `make start-test-environment`
    1. This bootstraps a local test environment with **Husky**'s dependencies using your `docker` and `docker-compose` daemons.
    2. It is a blocking operation, SIGTERM or SIGINT will kill it.
    3. It will feed off of the properties in `./application.yaml`
        1. MessageBroker GUIs
            1. https://www.conduktor.io
            2. https://www.kafkatool.com
            3. (CLI) https://vectorized.io/redpanda
        2. DB GUIs
            1. https://github.com/tarantool/awesome-tarantool#gui-clients
            2. (CLI) `docker exec -t -i mytarantool console` where `mytarantool` is the container name
5. `make all`
    1. This runs the CI pipeline, locally -- the same pipeline that PR checks run.
    2. Run it before you commit to save time & not wait for PR check to fail remotely.
6. `make local`
    1. This runs the CI pipeline, in a descriptive/debug mode. Run it before you run the "real" one --`make all`--.
    2. If this takes too much it might be because of `make buildMultiPlatformDockerImage` that builds docker images for `arm64 s390x ppc64le` as well. If that's
       the case, remove those 3 platforms from the iteration and retry (but don't commit!)
7. `make lint`
    1. This runs the linters. It is a part of the other pipelines, so you can run this separately to fix lint issues.
8. `make test`
    1. This runs all tests.
9. `make benchmark`
    1. This runs all benchmarks.
