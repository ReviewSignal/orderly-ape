# Orderly Ape

Orchestrate and run k6 load tests across multiple Kubernetes clusters. It consists of two main components:

1. A web application with a traditional API that allows configuring tests and coordinating between independent k6-operators in Kubernetes clusters.
2. A k6 operator that consumes the API, executes the load tests, and reports the state back to the web application. This allows the test runs to be coordinated by the webapp and exposed to the user.

## Table of Contents

-   [Architecture](#architecture)
-   [Installation](#installation)
-   [K6 Operator Flowchart](#flowchart)
-   [Contributing](#contributing)
-   [License](#license)

## Architecture

-   [webapp](webapp) - Web application that allows configuring tests and coordinating between independent k6-operators in Kubernetes clusters.
-   [k6-operator](k6-operator) - k6 operator that consumes the API, executes the load tests, and reports the state back to the web application.
-   [k6](k6) - k6 container image that can be used to run k6 tests in a Kubernetes cluster.

```mermaid
flowchart LR
    W[Orderly Ape Webapp]

    subgraph "Data"
    G[Grafana]
    I[InfluxDB]
    end

    subgraph "`<small>Asia Kubernetes Cluster</small>`"
    K6-AP["K6 Operator"]
    K6-AP --[ fa:fa-arrows-rotate ]--> K6-AP-JOB1["K6 Job 1"] ---> I
    K6-AP --[ fa:fa-arrows-rotate ]--> K6-AP-JOB2["K6 Job 2"] ---> I
    K6-AP --[ fa:fa-arrows-rotate ]--> K6-AP-JOBN["K6 Job N"] ---> I
    end

    subgraph "`<small>Europe Kubernetes Cluster</small>`"
    K6-EU["K6 Operator"]
    K6-EU --[ fa:fa-arrows-rotate ]--> K6-EU-JOB1["K6 Job 1"] ---> I
    K6-EU --[ fa:fa-arrows-rotate ]--> K6-EU-JOB2["K6 Job 2"] ---> I
    K6-EU --[ fa:fa-arrows-rotate ]--> K6-EU-JOBN["K6 Job N"] ---> I
    end

    subgraph "`<small>US Kubernetes Cluster</small>`"
    K6-US["K6 Operator"]
    K6-US --[ fa:fa-arrows-rotate ]--> K6-US-JOB1["K6 Job 1"] ---> I
    K6-US --[ fa:fa-arrows-rotate ]--> K6-US-JOB2["K6 Job 2"] ---> I
    K6-US --[ fa:fa-arrows-rotate ]--> K6-US-JOBN["K6 Job N"] ---> I
    end

    W --[ fa:fa-arrows-rotate ]--> K6-AP
    W --[ fa:fa-arrows-rotate ]--> K6-EU
    W --[ fa:fa-arrows-rotate ]--> K6-US

    W -- Links --> G
    G -- Exposes --> I
```

## Installation

### Pre-requisites

Before you install Orderly Ape you should install it's data stack, with a functional [Grafana](https://artifacthub.io/packages/helm/grafana/grafana)
instance that can connect and display data from a functional [InfluxDB](https://artifacthub.io/packages/helm/influxdata/influxdb2).

### Install the Orderly Ape

Orderly Ape publishes helm charts as OCI images in the GitHub Container Registry. You can check releases on the GitHub
repository [releases page](https://github.com/ReviewSignal/orderly-ape/releases).

#### Install the webapp

```bash
helm upgrade -i orderly-ape oci://ghcr.io/reviewsignal/orderly-ape/charts/orderly-ape v0.1.0
```

For more details, check the [webapp chart docs](deploy/charts/webapp/README.md).

#### Install the k6-operator

You should install the operator in all the Kubernetes clusters you want to run tests on. For compatibility with the
webapp, you shoud install the same version of the k6-operator as the webapp.

Also, for each operator, you should create a separate account on the webapp, and place the account in the `Workers`
group.

```bash
helm upgrade -i orderly-ape-k6-operator oci://ghcr.io/reviewsignal/orderly-ape/charts/k6-operator v0.1.0
```

For more details, check the [k6-operator chart docs](deploy/charts/k6-operator/README.md).

### k6-operator state diagram

In a Kubernetes cluster, the k6-operator orchestrates test runs and reports back to the webapp. The following shows the states a test goes through.

```mermaid
flowchart TD
%% States
    PENDING([PENDING])
    QUEUED
    READY
    RUNNING
    COMPLETED([COMPLETED])
    FAILED([FAILED])
    CANCELED([CANCELED])
%% Transitions
    PENDING -- accept() --> QUEUED
    QUEUED -- fa:fa-pause wait for pods --> READY
    READY -- pods ready, start testing --> RUNNING
    RUNNING -- complete() --> COMPLETED
    PENDING -- fail() --> FAILED
    QUEUED -- fail() --> FAILED
    READY -- fail() --> FAILED
    RUNNING -- fail() --> FAILED
    PENDING -- cancel() --> CANCELED
    QUEUED -- cancel() --> CANCELED
    READY -- cancel() --> CANCELED
    RUNNING -- cancel() --> CANCELED
```

## License

This project is licensed under the terms of the MIT license. See the [LICENSE](LICENSE) file for details.
