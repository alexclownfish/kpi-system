#!/usr/bin/env python3
"""Exercise the real Docker Nginx proxy without application data or host ports."""
import argparse
import pathlib
import subprocess
import tempfile
import time
import uuid


def run(*args, check=True):
    return subprocess.run(args, check=check, capture_output=True, text=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", type=pathlib.Path, default=pathlib.Path(__file__).resolve().parents[1] / "nginx/conf.d/kpi.conf")
    parser.add_argument("--image", default="nginx:1.26.1")
    args = parser.parse_args()
    name = "kpi-origin-test-" + uuid.uuid4().hex[:12]
    harness = """events {}
http {
    access_log off;
    include /etc/nginx/conf.d/default.conf;
    server {
        listen 8080;
        listen 3000;
        location / {
            return 200 '$http_x_forwarded_proto://$http_host/api/download/exports/test';
        }
    }
}
"""
    try:
        with tempfile.TemporaryDirectory(prefix="kpi-origin-test-") as temporary:
            config = pathlib.Path(temporary) / "nginx.conf"
            config.write_text(harness)
            run("docker", "create", "--name", name, "--network", "none",
                "--add-host", "backend:127.0.0.1", "--add-host", "frontend:127.0.0.1",
                args.image, "nginx", "-g", "daemon off;")
            run("docker", "cp", str(args.config.resolve()), name + ":/etc/nginx/conf.d/default.conf")
            run("docker", "cp", str(config), name + ":/etc/nginx/nginx.conf")
            run("docker", "start", name)
            for _ in range(40):
                ready = run("docker", "exec", name, "curl", "-fsS", "http://127.0.0.1/health", check=False)
                if ready.returncode == 0:
                    break
                time.sleep(0.25)
            else:
                raise RuntimeError(run("docker", "logs", name).stderr)

            for host, forwarded, expected_scheme in [
                ("kpi.auracor.net:18182", "https", "https"),
                ("kpi.auracor.net", "https", "https"),
                ("localhost:8088", None, "http"),
                ("localhost:8088", "http", "http"),
                ("localhost:8088", "invalid", "http"),
            ]:
                for path in ("/api/origin", "/api/events/stream", "/health", "/"):
                    command = ["docker", "exec", name, "curl", "-fsS",
                               "-H", "Host: " + host]
                    if forwarded:
                        command += ["-H", "X-Forwarded-Proto: " + forwarded]
                    actual = run(*command, "http://127.0.0.1" + path).stdout
                    expected = expected_scheme + "://" + host + "/api/download/exports/test"
                    assert actual == expected, f"{path} forwarded={forwarded!r}: {actual!r} != {expected!r}"
            print("PASS: 20 real Nginx proxy checks; HTTPS/port retained and direct HTTP fallback correct")
    finally:
        run("docker", "rm", "-f", name, check=False)


if __name__ == "__main__":
    main()
