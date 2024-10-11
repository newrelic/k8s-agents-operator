<a href="https://opensource.newrelic.com/oss-category/#community-plus"><picture><source media="(prefers-color-scheme: dark)" srcset="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/dark/Community_Plus.png"><source media="(prefers-color-scheme: light)" srcset="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/Community_Plus.png"><img alt="New Relic Open Source community plus project banner." src="https://github.com/newrelic/opensource-website/raw/main/src/images/categories/Community_Plus.png"></picture></a>

# K8s Agents Operator [![codecov](https://codecov.io/gh/newrelic/k8s-agents-operator/graph/badge.svg?token=YUSEXVY3WF)](https://codecov.io/gh/newrelic/k8s-agents-operator)

This project auto-instruments containerized workloads in Kubernetes with New Relic agents.

## Table Of Contents

- [Installation](#installation)
- [Support](#support)
- [Contribute](#contribute)
- [License](#license)

## Installation

For instructions on how to install the Helm chart, read the [chart's README](./charts/k8s-agents-operator/README.md)

## Development

We use Minikube and Tilt to spawn a local environment that it will reload after any changes inside the charts or the integration code.

Make sure you have these tools or install them:

* [Install minikube](https://minikube.sigs.k8s.io/docs/start/)
* [Install Tilt](https://docs.tilt.dev/install.html)
* [Install Helm](https://helm.sh/docs/intro/install/)

Start the local environment:

```bash
ctlptl create registry ctlptl-registry --port=5005
ctlptl create cluster minikube --registry=ctlptl-registry
tilt up
```

## Support

New Relic hosts and moderates an online forum where you can interact with New Relic employees as well as other customers to get help and share best practices. Like all official New Relic open source projects, there's a related Community topic in the New Relic Explorers Hub. You can find this project's topic/threads here:

* [New Relic Documentation](https://docs.newrelic.com): Comprehensive guidance for using our platform
* [New Relic Community](https://forum.newrelic.com/t/new-relic-kubernetes-open-source-integration/109093): The best place to engage in troubleshooting questions
* [New Relic Developer](https://developer.newrelic.com/): Resources for building a custom observability applications
* [New Relic University](https://learn.newrelic.com/): A range of online training for New Relic users of every level
* [New Relic Technical Support](https://support.newrelic.com/) 24/7/365 ticketed support. Read more about our [Technical Support Offerings](https://docs.newrelic.com/docs/licenses/license-information/general-usage-licenses/support-plan)

## Contribute

We encourage your contributions to improve *K8s Agents Operator*! Keep in mind that when you submit your pull request, you'll need to sign the CLA via the click-through using CLA-Assistant. You only have to sign the CLA one time per project.

If you have any questions, or to execute our corporate CLA (which is required if your contribution is on behalf of a company), drop us an email at opensource@newrelic.com.

**A note about vulnerabilities**

As noted in our [security policy](../../security/policy), New Relic is committed to the privacy and security of our customers and their data. We believe that providing coordinated disclosure by security researchers and engaging with the security community are important means to achieve our security goals.

If you believe you have found a security vulnerability in this project or any of New Relic's products or websites, we welcome and greatly appreciate you reporting it to New Relic through [HackerOne](https://hackerone.com/newrelic).

If you would like to contribute to this project, review [these guidelines](./CONTRIBUTING.md).

To all contributors, we thank you!  Without your contribution, this project would not be what it is today.

## License
*K8s Agents Operator* is licensed under the [Apache 2.0](http://apache.org/licenses/LICENSE-2.0.txt) License.

