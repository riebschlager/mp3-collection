# Go Scripts for MP3 Collection

This folder contains Go implementations of the data extraction and build scripts for the MP3 Collection project.

## Setup Instructions for macOS

### 1. Install Go

If you haven't installed Go yet, you can do so using Homebrew or by downloading the installer from the official website.

**Option A: Using Homebrew (Recommended)**
```bash
brew install go
```

**Option B: Official Installer**
Download the macOS package from [go.dev/dl](https://go.dev/dl/) and run the installer.

### 2. Verify Installation

Open your terminal and run:
```bash
go version
```
You should see output similar to `go version go1.21.0 darwin/arm64`.

### 3. Setup Workspace (Optional)

Go generally works best with modules (which this project uses). You don't need to set up a `GOPATH` for modern Go development.

## Running the Scripts

The scripts are consolidated into a single Go application with subcommands.

### One-shot command
To run the equivalent of the `run_all.sh` Python script:

```bash
./run_all.sh
```

### Running individual commands manually

You can run individual steps using `go run .`:

```bash
# Extract tracks
go run . extract-tracks

# Extract artists
go run . extract-artists

# Extract albums
go run . extract-albums

# Build web data
go run . build-web-data
```

### Building the binary

For faster execution, you can build the binary once:

```bash
go build -o mp3-scripts
./mp3-scripts extract-tracks
```
