/*
Package cocoa provides interfaces to interact with groups of containers (called
pods) backed by container orchestration services. Containers are not managed
individually - they're managed as logical groupings of containers.

The ECSPodCreator provides an abstraction to create pods in AWS ECS without
needing to make direct calls to the API.

The ECSPod is a self-contained unit that allows users to manage their pod
without having to make direct calls to the AWS ECS API.

The ECSClient provides a convenience wrapper around the AWS ECS API. If the
ECSPodCreator and ECSPod do not fulfill your needs, you can instead make calls
directly to the ECS API using this client.

The Vault is an ancillary service for pods that supports interacting with a
dedicated secrets management service. It conveniently integrates with pods to
securely pass secrets into containers.

The SecretsManagerClient provides a convenience wrapper around the AWS Secrets
Manager API. If the Vault does not fulfill your needs, you can instead make
calls directly to the Secrets Manager API using this client.
*/
package cocoa
