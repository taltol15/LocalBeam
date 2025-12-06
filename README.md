# 📡 LocalBeam
![Downloads](https://img.shields.io/github/downloads/taltol15/LocalBeam/total?style=for-the-badge&color=blue)
> **Fast. Secure. Local.**
> The easiest way to transfer files between devices on your local network. No internet required.

![LocalBeam Screenshot](https://localbeam.net/app-screen.png) 


## 🚀 Overview

**LocalBeam** is a modern, lightweight desktop application designed to solve a simple problem: moving files between computers on the same Wi-Fi/LAN without using the cloud, email, or USB drives.

Built with **Go** (backend) and **React** (frontend) using the **Wails** framework, it combines native performance with a beautiful modern UI.

## ✨ Features

* **🔍 Auto-Discovery:** Automatically finds other devices running LocalBeam on your network using UDP broadcast.
* **⚡ Blazing Fast:** Transfers files directly over LAN (peer-to-peer). Speed is only limited by your Wi-Fi.
* **🔒 Secure:** Every transfer requires a dynamic **Security PIN**. No unwanted files.
* **📉 Real-time Progress:** Visual progress bars for both sender and receiver.
* **🌐 Offline First:** No internet connection needed. Your data never leaves your local network.
* **💾 Smart Memory:** Handles large files (GBs) efficiently without crashing RAM.

## 📥 Download

Get the latest version for Windows from the **[Releases Page](../../releases)**.

## 🛠️ Tech Stack

* **Backend:** Go (Golang)
* **Frontend:** React + Vite
* **Framework:** Wails v2
* **Styling:** CSS3 (Custom Dark Mode Theme)

## 🚀 How to Run (for Developers)

If you want to build it yourself:

1.  Install [Go](https://go.dev/) and [Node.js](https://nodejs.org/).
2.  Install Wails: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
3.  Clone the repo:
    ```bash
    git clone [https://github.com/taltol15/LocalBeam.git](https://github.com/taltol15/LocalBeam.git)
    cd LocalBeam
    ```
4.  Run in dev mode:
    ```bash
    wails dev
    ```
5.  Build for production:
    ```bash
    wails build
    ```

## 📝 License

This project is open-source and available under the **MIT License**.

---
*Developed with ❤️ by Tal*
