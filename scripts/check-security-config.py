#!/usr/bin/env python3
"""Keep local-only network and secret-safe gateway defaults from regressing."""

from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def published_ports(compose: str) -> list[str]:
    ports: list[str] = []
    in_ports = False
    ports_indent = 0
    for line in compose.splitlines():
        stripped = line.strip()
        indent = len(line) - len(line.lstrip())
        if stripped == "ports:":
            in_ports = True
            ports_indent = indent
            continue
        if in_ports and stripped and indent <= ports_indent:
            in_ports = False
        if in_ports and stripped.startswith("- "):
            ports.append(stripped[2:].strip('"'))
    return ports


def main() -> int:
    failures: list[str] = []
    compose = (ROOT / "deployments/compose/compose.yaml").read_text(encoding="utf-8")
    ports = published_ports(compose)
    if not ports or any(not value.startswith("127.0.0.1:") for value in ports):
        failures.append(f"Compose published ports must all bind loopback: {ports}")
    if "${HUBCR_REGISTRY_EVENT_TOKEN:?" not in compose:
        failures.append("Compose must fail when the Registry event token is missing")

    distribution = (ROOT / "deployments/compose/distribution/config.yml").read_text(encoding="utf-8")
    if "Authorization: []" not in distribution:
        failures.append("Distribution configuration must not contain an accepted event token")

    environment = (ROOT / ".env.example").read_text(encoding="utf-8")
    if "HUBCR_API_ADDRESS=127.0.0.1:8080" not in environment:
        failures.append("example API listener must bind loopback")
    if "HUBCR_REGISTRY_EVENT_TOKEN=\n" not in environment:
        failures.append("example environment must not contain an event token value")

    gateway = (ROOT / "deployments/compose/gateway/default.conf.template").read_text(encoding="utf-8")
    before_registry, marker, registry_and_after = gateway.partition("    location /v2/ {")
    registry_location, end_marker, _ = registry_and_after.partition("\n    }")
    if not marker or not end_marker:
        failures.append("gateway Registry location could not be identified")
    else:
        for unsafe_global in ("proxy_request_buffering off", "proxy_buffering off", "900s"):
            if unsafe_global in before_registry:
                failures.append(f"Registry streaming directive is global: {unsafe_global}")
            if unsafe_global not in registry_location:
                failures.append(f"Registry location lost required streaming directive: {unsafe_global}")

    if failures:
        print("\n".join(failures))
        return 1
    print("Local network, event-secret, and gateway timeout defaults are hardened")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
