#!/usr/bin/env python3

from http.server import BaseHTTPRequestHandler, HTTPServer
import json

# Define the hostname and port to listen on
hostname = "localhost"
port = 8080

class MyServer(BaseHTTPRequestHandler):
    def do_GET(self):
        
        if self.path == "/api/deployments/lb-config-ng":
            self.send_response(200)
            self.send_header("Content-type", "application/json")
            self.end_headers()
            response = {
                "http": {
                    "routers": {
                        "finer-snail-230f97.flakery.xyz": {"service": "230f97a2-8e84-4d9b-8246-11caf8e4507a"},
                    },
                    "services": {
                        "230f97a2-8e84-4d9b-8246-11caf8e4507a": {"servers": [{"url": "http://10.0.2.112:8080"}, {"url": "http://10.0.2.12:8080"}]},
                    },
                },
            }
            self.wfile.write(json.dumps(response).encode())

        else:
            self.send_response(404)
            self.send_header("Content-type", "text/plain")
            self.end_headers()
            self.wfile.write(b"Path not found")

if __name__ == "__main__":

    webServer = HTTPServer((hostname, port), MyServer)
    print(f"Server started http://{hostname}:{port}")

    try:
        webServer.serve_forever()
    except KeyboardInterrupt:
        pass

    webServer.server_close()
    print("Server stopped.")
