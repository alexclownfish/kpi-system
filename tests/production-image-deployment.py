#!/usr/bin/env python3
"""Verify image-only deployment without touching existing projects or volumes."""
import argparse
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
TEMPLATE = ROOT / "deploy/production/docker-compose.yml"
ENV = {k: v for k, v in os.environ.items() if not k.startswith("COMPOSE_")}
VOLUMES = ("kpi-postgres-data", "kpi-db", "kpi-exports", "kpi-signoffs", "kpi-backups")


def run(args, *, data=None, env=None, ok=True):
    result = subprocess.run(args, input=data, text=True, capture_output=True, env=env or ENV)
    if ok and result.returncode:
        # Do not include stdout: login responses and Compose config contain secrets.
        raise RuntimeError(f"Command failed: {args[:3]}\n{result.stderr[-2000:]}")
    return result


def compose_config(folder):
    args = ["docker", "compose", "--project-directory", str(folder),
            "--env-file", str(folder / ".env"), "-f", str(TEMPLATE)]
    result = run(args + ["config", "--format", "json"])
    return json.loads(result.stdout), args


def verify_config(folder):
    config, args = compose_config(folder)
    # Reproduce the reported deployment mistake: the old base file alone
    # omits the DB driver, which makes the application select SQLite.
    legacy_args = args[:-1] + [str(ROOT / "docker-compose.yml")]
    legacy = json.loads(run(legacy_args + ["config", "--format", "json"]).stdout)
    assert "DB_DRIVER" not in legacy["services"]["backend"]["environment"]
    assert config["name"] == "kpi-system-pg"
    services = config["services"]
    assert set(services) == {"postgres", "backend", "frontend", "nginx-gateway"}
    assert all("build" not in s for s in services.values())
    backend = services["backend"]
    assert backend["environment"]["DB_DRIVER"] == "postgres"
    assert backend["environment"]["PGHOST"] == "postgres"
    assert backend["environment"]["PGUSER"] == "kpi_app"
    assert backend["environment"]["BOOTSTRAP_ADMIN_EMAIL"] == ""
    assert backend["depends_on"]["postgres"]["condition"] == "service_healthy"
    assert not services["postgres"].get("ports")
    assert not backend.get("ports")
    assert not any("pg-admin-secrets" in str(v) for v in backend["volumes"])
    for name in ("backend", "frontend", "nginx-gateway"):
        assert services[name]["pull_policy"] == "always"
    for name in VOLUMES:
        assert config["volumes"][name]["external"] is True
        assert config["volumes"][name]["name"] == f"kpi-system-pg_{name}"
    mounts = {v.get("source"): v["target"] for v in backend["volumes"]}
    assert mounts["kpi-exports"] == "/app/public/exports"
    assert mounts["kpi-signoffs"] == "/app/uploads/evaluation-confirmations"
    assert mounts[str(folder / "secrets")] == "/run/kpi-secrets"
    # Missing mandatory deployment values must fail before contacting Docker.
    original = (folder / ".env").read_text()
    try:
        for name in ("JWT_SECRET", "CORS_ALLOWED_ORIGINS"):
            (folder / ".env").write_text("\n".join(
                line for line in original.splitlines() if not line.startswith(name + "=")))
            env = {k: v for k, v in ENV.items() if k != name}
            assert run(args + ["config", "--quiet"], env=env, ok=False).returncode != 0
    finally:
        (folder / ".env").write_text(original)
    print("PASS: standalone PG config, existing external volumes, export mount, missing secrets")
    return config


def verify_build_cli(folder):
    fixture = folder / "build-fixture"
    (fixture / "scripts").mkdir(parents=True)
    (fixture / "bin").mkdir()
    shutil.copy(ROOT / "scripts/build-docker-images.sh", fixture / "scripts")
    log = fixture / "calls.log"
    for name, source in {
        "git": '#!/bin/sh\nprintf "git %s\\n" "$*" >> "$TEST_CALLS"\n'
               'case "$*" in *--short*) echo abc123;; esac\n',
        "docker": '#!/bin/sh\nprintf "docker %s\\n" "$*" >> "$TEST_CALLS"\n',
    }.items():
        executable = fixture / "bin" / name
        executable.write_text(source)
        executable.chmod(0o700)
    env = dict(ENV, PATH=str(fixture / "bin") + os.pathsep + ENV["PATH"], TEST_CALLS=str(log))
    run(["bash", str(fixture / "scripts/build-docker-images.sh"),
         "--prefix", "registry.example.com/team/kpi", "--tag", "release-test",
         "--platform", "linux/amd64", "--with-nginx", "--push"], env=env)
    calls = log.read_text().splitlines()
    assert calls.index("git pull --ff-only") < next(i for i, s in enumerate(calls) if s.startswith("docker build "))
    builds = [s for s in calls if s.startswith("docker build ")]
    assert len(builds) == 3
    for service in ("backend", "frontend", "nginx"):
        image = f"registry.example.com/team/kpi-{service}:release-test"
        assert any(f"-t {image} " in line and "--platform linux/amd64" in line for line in builds)
        assert f"docker push {image}" in calls
    assert not any("compose " in line for line in calls)
    print("PASS: existing build CLI pulls Git, builds and pushes three versioned images, never deploys")


def verify_runtime(folder, config):
    backend_image = os.environ.get("KPI_TEST_BACKEND_IMAGE")
    if not backend_image:
        raise RuntimeError("Set KPI_TEST_BACKEND_IMAGE to a locally built PG-capable backend image")
    project = "kpi-image-deploy-test-" + secrets.token_hex(6)
    for service, image in {
        "backend": backend_image,
        "frontend": os.environ.get("KPI_TEST_FRONTEND_IMAGE", "kpi-system-frontend:latest"),
        "nginx-gateway": os.environ.get("KPI_TEST_NGINX_IMAGE", "kpi-system-nginx:local"),
    }.items():
        run(["docker", "image", "inspect", image])
        config["services"][service]["image"] = image
        config["services"][service]["pull_policy"] = "never"
    run(["docker", "image", "inspect", "postgres:16-alpine"])
    config["services"]["postgres"]["pull_policy"] = "never"
    config["name"] = project
    config["networks"]["default"]["name"] = project + "_default"
    config["services"]["nginx-gateway"].pop("ports", None)  # No host ports.
    for name in VOLUMES:
        config["volumes"][name]["name"] = project + "_" + name
    (folder / "secrets").mkdir()
    (folder / "pg-admin-secrets").mkdir()
    password = secrets.token_urlsafe(32)
    for path, value in (
        (folder / "secrets/postgres_password", secrets.token_hex(32)),
        (folder / "pg-admin-secrets/postgres_admin_password", secrets.token_hex(32)),
        (folder / "secrets/bootstrap_admin_password", password),
    ):
        path.write_text(value + "\n")
        path.chmod(0o600)
    backend_env = config["services"]["backend"]["environment"]
    backend_env.update(BOOTSTRAP_ADMIN_EMAIL="image-deploy-test@example.invalid",
                       BOOTSTRAP_ADMIN_NAME="image-deploy-test",
                       BOOTSTRAP_ADMIN_PASSWORD_FILE="/run/kpi-secrets/bootstrap_admin_password")
    runtime = folder / "runtime.json"
    runtime.write_text(json.dumps(config))
    dc = ["docker", "compose", "-p", project, "-f", str(runtime)]
    created = []
    try:
        # The same external-volume template refuses to create an empty PG volume.
        rejected = run(dc + ["up", "-d", "postgres"], ok=False)
        assert rejected.returncode and "external" in rejected.stderr.lower()
        for volume in config["volumes"].values():
            assert run(["docker", "volume", "inspect", volume["name"]], ok=False).returncode != 0
            run(["docker", "volume", "create", volume["name"]])
            created.append(volume["name"])
        run(dc + ["up", "-d", "--wait", "--wait-timeout", "180", "postgres"])
        run(dc + ["exec", "-T", "--user", "0", "postgres", "sh", "-s"],
            data=(ROOT / "scripts/provision-postgres-role.sh").read_text())
        run(dc + ["up", "-d", "--wait", "--wait-timeout", "240"])

        def gateway(path, payload=None):
            args = dc + ["exec", "-T", "nginx-gateway", "curl", "-fsS", "--retry", "10",
                         "--retry-connrefused", "--retry-delay", "1"]
            if payload:
                args += ["-H", "Content-Type: application/json", "--data-binary", "@-"]
            return run(args + ["http://127.0.0.1" + path], data=payload).stdout

        assert json.loads(gateway("/health"))["status"] == "OK"
        assert "<html" in gateway("/auth/login").lower()
        login_payload = json.dumps({"email": "image-deploy-test@example.invalid", "password": password})
        first_login = json.loads(gateway("/api/auth/login", login_payload))
        assert first_login.get("token") or first_login.get("data", {}).get("token")
        assert first_login["user"]["role"] == "super_admin"
        marker = secrets.token_hex(16)
        run(dc + ["exec", "-T", "backend", "sh", "-ec",
                  'printf "%s" "$1" > /app/public/exports/deployment-test.txt; '
                  'printf "%s" "$1" > /app/uploads/evaluation-confirmations/deployment-test.txt',
                  "test-marker", marker])
        # Normal production config contains no bootstrap credentials.
        for name in ("BOOTSTRAP_ADMIN_EMAIL", "BOOTSTRAP_ADMIN_NAME", "BOOTSTRAP_ADMIN_PASSWORD_FILE"):
            backend_env[name] = ""
        runtime.write_text(json.dumps(config))
        run(dc + ["up", "-d", "--force-recreate", "--wait", "--wait-timeout", "240",
                  "backend", "frontend", "nginx-gateway"])
        assert json.loads(gateway("/health"))["status"] == "OK"
        second_login = json.loads(gateway("/api/auth/login", login_payload))
        assert second_login["user"]["id"] == first_login["user"]["id"]
        for path in ("/app/public/exports/deployment-test.txt",
                     "/app/uploads/evaluation-confirmations/deployment-test.txt"):
            assert run(dc + ["exec", "-T", "backend", "cat", path]).stdout == marker
        assert "<html" in gateway("/auth/login").lower()
        print("PASS: missing volume rejected; actual PG/app/gateway start, login and page work;")
        print("      original owner/password, signoff and export files survive app recreation without bootstrap")
    except Exception:
        logs = run(dc + ["logs", "--tail=25", "postgres"], ok=False)
        print(logs.stdout, flush=True)
        raise
    finally:
        run(dc + ["down"], ok=False)
        for name in created:
            run(["docker", "volume", "rm", name], ok=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--integration", action="store_true")
    args = parser.parse_args()
    # Colima normally shares the user's home; relocated checkouts and /tmp may
    # not be shared. The parent can be overridden for other Docker hosts.
    scratch_parent = Path(os.environ.get("KPI_TEST_SCRATCH_PARENT", Path.home()))
    with tempfile.TemporaryDirectory(prefix=".image-deploy-test-", dir=scratch_parent) as scratch:
        folder = Path(scratch)
        (folder / ".env").write_text(
            "JWT_SECRET=" + secrets.token_urlsafe(48) + "\n"
            "CORS_ALLOWED_ORIGINS=http://localhost\n"
            "DB_DRIVER=sqlite\nBOOTSTRAP_ADMIN_EMAIL=ignored@example.invalid\n")
        config = verify_config(folder)
        verify_build_cli(folder)
        if args.integration:
            verify_runtime(folder, config)
        else:
            print("Runtime checks not run; use --integration with KPI_TEST_BACKEND_IMAGE")


if __name__ == "__main__":
    main()
