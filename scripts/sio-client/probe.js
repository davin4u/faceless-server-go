// Tiny probe: connect, register stub auth, emit one event, exit.
// Usage: node probe.js <serverUrl> <publicKeyB64> <secretKeyB64>
const { io } = require('socket.io-client');
const nacl = require('tweetnacl');
const { encodeBase64, decodeBase64, decodeUTF8 } = require('tweetnacl-util');

const [, , url, pubB64, secB64] = process.argv;
const ts = String(Math.floor(Date.now() / 1000));
const sig = encodeBase64(
  nacl.sign.detached(decodeUTF8(ts), decodeBase64(secB64))
);

const socket = io(url, {
  auth: { publicKey: pubB64, timestamp: ts, signature: sig, socketType: 'app' },
  transports: ['websocket'],
});

socket.on('connect', () => {
  console.log(JSON.stringify({ event: 'connect', id: socket.id }));
  setTimeout(() => process.exit(0), 250);
});
socket.on('connect_error', (err) => {
  console.log(JSON.stringify({ event: 'connect_error', message: err.message }));
  process.exit(1);
});
