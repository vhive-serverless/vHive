FROM amd64/debian:buster-slim AS runner

# ca-certificates are required if you are going to interact with any
# HTTPS endpoints (e.g. AWS).
# nano and tmux are installed for debugging if and when needed.
RUN apt-get update && apt-get install -y ca-certificates nano tmux
