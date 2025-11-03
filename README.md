<img width="1280" alt="Code Server" src="https://github.com/code-payments/code-server/assets/5760385/a7f19b1a-8052-422c-88c6-ad68cd86eb6d">

# Code Server

[![Release](https://img.shields.io/github/v/release/code-payments/code-server.svg)](https://github.com/code-payments/code-server/releases/latest)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/code-payments/code-server)](https://pkg.go.dev/github.com/code-payments/code-server/pkg)
[![Tests](https://github.com/code-payments/code-server/actions/workflows/test.yml/badge.svg)](https://github.com/code-payments/code-server/actions/workflows/test.yml)
[![GitHub License](https://img.shields.io/badge/license-MIT-lightgrey.svg?style=flat)](https://github.com/code-payments/code-server/blob/main/LICENSE.md)

Code server monolith containing the gRPC/web services and workers that power a next-generation payments system. The project contains the first L2 solution on top of Solana, utilizing an intent-based system backed by the Code Sequencer to handle transactions.

## What is Flipcash?

[Flipcash](https://flipcash.com) is a mobile wallet app leveraging self-custodial blockchain technology to provide an instant, global, and private payments experience. We are currently working on a currency launchpad.

## Quick Start

1. Install Go. See the [official documentation](https://go.dev/doc/install).

2. Download the source code.

```bash
git clone git@github.com:code-payments/code-server.git
```

3. Run the test suite:

```bash
make test
```

## Project Structure

The implementations powering the Code ecosystem (Flipcash App, SDK, etc) can be found under the `pkg/code/` directory. All other code under the `pkg/` directory are generic libraries and utilities.

To begin diving into core systems, we recommend starting with the following packages:
- `pkg/code/async/`: Asynchronous workers that perform tasks outside of RPC and web calls
- `pkg/code/server/`: gRPC and web service implementations

## APIs

The gRPC APIs provided by Code server can be found in the [code-protobuf-api](https://github.com/code-payments/code-protobuf-api) project.

## Contributing

Anyone is welcome to make code contributions through a PR.

This will evolve as we continue to build out the platform and open up more ways to contribute.

## Getting Help

If you have any additional questions or need help integrating Code into your website or application, please reach out to us on [Twitter](https://twitter.com/flipcash).
