# -*- mode: Python -*-

#### Config
# This env var is automatically added by the e2e action.
namespace = os.getenv('NAMESPACE','newrelic')
chart_values_file = 'local/super-agent-tilt.yml'


#### Build the final Docker image with the binary.
docker_build(
  'tilt.local/operator-dev',
  context='.',
  dockerfile='./Dockerfile'
)


#### Set-up charts
load('ext://helm_resource', 'helm_repo','helm_resource')
load('ext://git_resource', 'git_checkout')

update_dependencies = True
chart = 'charts/k8s-agents-operator'
deps=[chart]


flags_helm = ['--create-namespace','--version=>=0.0.0-beta','--set=super-agent-deployment.image.imagePullPolicy=Always','--values=' + chart_values_file]


#### Installs charts
helm_resource(
  'operator',
  chart,
  deps=deps, # re-deploy chart if modified locally
  namespace=namespace,
  release_name='operator',
  update_dependencies=False,
  flags=flags_helm,
  image_deps=['tilt.local/operator-dev'],
  image_keys=[('controllerManager.manager.image.repository', 'controllerManager.manager.image.tag')],
  resource_deps=[]
)

update_settings(k8s_upsert_timeout_secs=150)

