
# GoAudioStreamer

## Low-Latency Voice Chat Server

A simple, efficient Golang-based client-server application for voice communication during gaming sessions, designed for 2-4 friends with minimal bandwidth and resource usage.

### Project Story

This project was born out of necessity during a tough time. The last week of October, I came down with a severe flu—running a fever of 40°C (104°F)—which left me unable to focus on work or studies. To pass the time and stay connected, I turned to playing PC games with my friends. However, my aging PC struggled with third-party apps like ***Discord*** for voice chat; it caused noticeable FPS drops and consumed more bandwidth than necessary, making gameplay laggy and frustrating.
This project was born out of necessity during a tough time. The last week of October, I came down with a severe flu—running a fever of 40°C (104°F)—which left me unable to focus on work or studies. To pass the time and stay connected, I turned to playing PC games with my friends. However, my aging PC struggled with third-party apps like ***Discord*** for voice chat; it caused noticeable FPS drops and consumed more bandwidth than necessary, making gameplay laggy and frustrating.

Frustrated with these issues, I decided to build a lightweight alternative: a terminal-based voice chat tool optimized for low latency and minimal resources. The goal was to enable seamless talking with 2-4 friends while gaming, without the overhead of bloated applications, reducing bandwidth to essentials and keeping CPU/GPU impact near zero.

### How It Works

This app uses a central server to handle audio mixing and distribution, with clients capturing microphone input and playing back received audio. Key features:

- ***Audio Handling***: Captures mono audio from the default microphone at 48kHz using PortAudio. Compresses it with the Opus codec at ~12kbps for low-bandwidth voice transmission.
- ***Server Mixing***: Up to 4 clients connect via TCP. The server decodes incoming audio, mixes frames (summing PCM samples with clipping to avoid distortion), re-encodes the mix, and broadcasts it back—only when 2+ clients are connected to save resources.
- ***Efficiency Features***: DTX (Discontinuous Transmission) skips silent/noisy frames to reduce data sent.
- Simple password authentication ensures only trusted friends join.
- ***Latency Reduction***: 20ms frames minimize delay; jitter buffering on clients smooths playback without underruns.

### Setup

Run the server with:

```bash
go run server.go -password=<pass>
```

Clients connect via:

```bash
go run client.go -host=<ip> -port=1234 -password=<pass>
```

Cross-compiled for Windows for easy sharing—no dependencies needed.

For full code and dependencies, see the source files. Contributions welcome to add features like auto-reconnect or UDP support!

## Cross compile for windows

```bash

sudo apt install libportaudio2 libportaudiocpp0 libportaudio-ocaml-dev  libopus-dev libasound2-dev libogg-dev libopusfile-dev libopus-dev libasound2-dev git build-essential pkg-config cmake autoconf libtool gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64 mingw-w64-tools mingw-w64-x86-64-dev  -y

mkdir $HOME/mingw-libs
mkdir -p /opt/SP/misc
cd $_
wget http://files.portaudio.com/archives/pa_stable_v190700_20210406.tgz && tar -xzf pa_stable_v190700_20210406.tgz && rm pa_stable_v190700_20210406.tgz
wget https://downloads.xiph.org/releases/ogg/libogg-1.3.6.tar.gz && tar -xzf libogg-1.3.6.tar.gz && rm libogg-1.3.6.tar.gz
wget https://downloads.xiph.org/releases/opus/opus-1.5.2.tar.gz && tar -xzf opus-1.5.2.tar.gz  && rm  opus-1.5.2.tar.gz

cd portaudio
sudo make uninstall
make clean
./configure --enable-static --disable-shared #--prefix=$HOME/mingw-libs --host=x86_64-w64-mingw32 
make -j$(nproc)
sudo make install
sudo ldconfig
sudo /usr/bin/install -c -d /usr/local/include
for include in portaudio.h pa_linux_alsa.h pa_jack.h; do sudo /usr/bin/install -c -m 644 -m 644 ./include/$include /usr/local/include/$include; done
sudo /usr/bin/install -c -d /usr/local/lib/pkgconfig
sudo /usr/bin/install -c -m 644 portaudio-2.0.pc /usr/local/lib/pkgconfig/portaudio-2.0.pc
cd ..

cd libogg-1.3.6
sudo make uninstall
make clean
./configure --enable-static --disable-shared #--prefix=$HOME/mingw-libs  --host=x86_64-w64-mingw32
make -j$(nproc)
sudo make install
sudo make install
cd ..

cd opus-1.5.2
sudo make uninstall
make clean
./configure --enable-static --disable-shared #--with-winapi=wasapi,wmme,directsound --prefix=$HOME/mingw-libs --host=x86_64-w64-mingw32 
make -j$(nproc)
sudo make install
sudo ldconfig
cd ..

git clone https://github.com/xiph/rnnoise.git
sudo make uninstall
cd rnnoise
./autogen.sh
./configure  --enable-static --disable-shared #CFLAGS='-O3 -march=x86-64' CXXFLAGS='-O3 -march=x86-64' --host=x86_64-w64-mingw32 --prefix=$HOME/mingw-libs
make -j$(nproc)
sudo make install
```

### Build

```bash
bash build.sh linux
bash build.sh windows
```

Binaries are in `./bin`
