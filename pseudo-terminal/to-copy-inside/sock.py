# echo-server.py
import time
import socket

print("socket test main.py")

HOST = "0.0.0.0"  # Standard loopback interface address (localhost)
HOST = "127.0.0.1"  # Standard loopback interface address (localhost)
PORT = 7070  # Port to listen on (non-privileged ports are > 1023)

with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
    s.bind((HOST, PORT))
    s.listen()
    conn, addr = s.accept()
    while conn:
        print(f"Connected by {addr}")
        time.sleep(3)
        while True:
            data = conn.recv(1024)
            if not data:
                break
            print(data) ## 
            conn.sendall(data)

