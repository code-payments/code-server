<img width="1280" alt="Code Server" src="https://github.com/code-payments/code-server/assets/5760385/a7f19b1a-8052-422c-88c6-ad68cd86eb6d">

# Code Server

[![Release](https://img.shields.io/github/v/release/code-payments/code-server.svg)](https://github.com/code-payments/code-server/releases/latest)
[![PkgGoDev](https://pkg.go.dev/badge/github.com/code-payments/code-server)](https://pkg.go.dev/github.com/code-payments/code-server/pkg)
[![Tests](https://github.com/code-payments/code-server/actions/workflows/test.yml/badge.svg)](https://github.com/code-payments/code-server/actions/workflows/test.yml)
[![GitHub License](https://img.shields.io/badge/license-MIT-lightgrey.svg?style=flat)](https://github.com/code-payments/code-server/blob/main/LICENSE.md)

Code server monolith containing the gRPC/web services and workers that power a next-generation payments system. The project contains the first L2 solution on top of Solana, utilizing an intent-based system backed by the Code Sequencer to handle transactions.

## What is Code?

[Code](https://getcode.com) is a mobile wallet app leveraging self-custodial blockchain technology to provide an instant, global, and private payments experience.

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

The implementations powering the Code ecosystem (Code Wallet App, Code SDK, etc) can be found under the `pkg/code/` directory. All other code under the `pkg/` directory are generic libraries and utilities.

To begin diving into core systems, we recommend starting with the following packages:
- `pkg/code/async/`: Asynchronous workers that perform tasks outside of RPC and web calls
- `pkg/code/server/`: gRPC and web service implementations

## APIs

The gRPC APIs provided by Code server can be found in the [code-protobuf-api](https://github.com/code-payments/code-protobuf-api) project. Refer to the [Code SDK](https://github.com/code-payments/code-sdk) if you want to integrate micropayments into your web application.

## Learn More

To learn more about fundamental concepts and advanced topics related to Code server, we recommend jumping into the Code SDK [documentation](https://code-payments.github.io/code-sdk/docs/guide/introduction) for a high-level overview. The Extra Topics section includes notes on topics like the Sequencer, Privacy Protocol, Timelock, and more.

## Contributing

Anyone is welcome to make code contributions through a PR.

The best way to share general feedback is on [Discord](https://discord.gg/T8Tpj8DBFp).

This will evolve as we continue to build out the platform and open up more ways to contribute.

## Getting Help

If you have any additional questions or need help integrating Code into your website or application, please reach out to us on [Discord](https://discord.gg/T8Tpj8DBFp) or [Twitter](https://twitter.com/getcode).

### Using custom Swap API endpoints

You can set custom URLs via the configuration for any self-hosted Jupiter APIs, like the [V6 Swap API](https://station.jup.ag/docs/apis/self-hosted) or [Paid Hosted APIs](https://station.jup.ag/docs/apis/self-hosted#paid-hosted-apis) Here is an example:

```
JUPITER_API_BASE_URL=https://quote-api.jup.ag/v6/
```