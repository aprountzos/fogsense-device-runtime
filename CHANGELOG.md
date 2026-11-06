# Changelog

All notable FogSense Device Runtime changes are documented here.

The format is based on Keep a Changelog (https://keepachangelog.com/en/1.1.0), and releases follow Semantic Versioning (https://semver.org/).

## [Unreleased]

### Added
- Alternative MQTT 5.0 gateway transport; HTTP remains the default.
- Per-device MQTT topics for registration, telemetry, events, commands and acknowledgements.
- MQTT username/password support and TLS broker URLs.
- Local Mosquitto + mock MQTT gateway Docker Compose playground.
- MQTT gateway contract and Docusaurus guide.
- CI checks for Go tests, vet/build, Docker image build and Docusaurus production build.

### Changed
- Separate HTTP gateway authentication (GATEWAY_TOKEN) from the local device API token (DEVICE_TOKEN), retaining the previous fallback behavior.

### Notes
- MQTT is an alternative gateway transport; the device's local HTTP API remains unchanged.
- The included Mosquitto configuration allows anonymous access for local development only.
- This is unreleased development functionality. See the feature branch and pull request for implementation status.
