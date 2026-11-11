# Set up local AI with Ollama

ChartNagari's AI setup wizard helps connect a local or remote Ollama model. It
does not silently install Ollama: a browser cannot install software on the
computer that opened the page. Install Ollama on the selected host yourself
with the [official Ollama download](https://ollama.com/download).

## Choose where Ollama runs

- **ChartNagari installation device** runs Ollama on the machine running the
  ChartNagari Go server. With Docker, that means the server/container host, not
  necessarily your browser computer.
- **Remote Ollama server** uses an Ollama instance already running on another
  machine. Enter an endpoint reachable from the ChartNagari server. A `localhost`
  address always refers to the machine/container making the connection.

For a Docker-based ChartNagari deployment, the Ollama status card may offer
**Enable Docker sidecar**. Choose it to write the Compose override, then run the
displayed `docker compose up -d ollama` command from the ChartNagari project
directory. Return to the wizard and wait for the sidecar status to become
available before saving the model profile. This starts the Ollama container;
the model still needs to be downloaded explicitly in the profile flow below.

## First-time setup

1. Open **Settings → AI** or choose AI setup during onboarding. Select the
   ChartNagari host or a remote Ollama host.
2. If Ollama is not installed on the selected host, use the official installer
   link shown by the wizard. After installation, return to ChartNagari and
   refresh status. When Ollama is installed but stopped, use **Start** to start
   its local service.
3. Choose a model preset, such as Qwen3, and save the profile. For a local
   Ollama endpoint, an API key is normally not required; remote servers may
   require authentication according to their configuration.
4. Choose **Download model** to pull the model onto that Ollama host. Model files
   can be several gigabytes; this is a separate, explicit action.
5. Run the sample response test, review the output, and activate the profile.
   Saving or downloading alone does not activate it.

The wizard can list models already installed on the selected Ollama host. A
profile's saved credentials remain on the ChartNagari server. Protect its
configuration and backups.

## Troubleshooting

- **Not installed:** confirm that you installed Ollama on the host selected in
  the wizard, rather than only on the browser computer. Refresh status.
- **Installed but unavailable:** use **Start** for the ChartNagari host, or
  start/check the Ollama service on the remote machine. Verify the endpoint is
  reachable from the ChartNagari server.
- **Model missing:** save the intended profile and explicitly download that
  model. Confirm sufficient disk space and wait for the pull to finish.
- **Test fails:** check the endpoint, model identifier, remote authentication,
  host logs, and available memory. The wizard does not silently fall back to a
  hosted or paid provider.

See [AI connection settings](../SETTINGS.md#ai-connection-setup) for profile,
credential, and activation details.
