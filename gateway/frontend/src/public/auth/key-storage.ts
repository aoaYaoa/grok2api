import { publicKeyStorage } from "@/public/api/contracts.mjs";

const encPrefix = "enc:v1:";
const xorPrefix = "enc:xor:";
const secret = "grok2api-admin-key";
const encoder = new TextEncoder();
const decoder = new TextDecoder();

function bytesToBase64(bytes: Uint8Array) { return btoa(String.fromCharCode(...bytes)); }
function base64ToBytes(value: string) { return Uint8Array.from(atob(value), (char) => char.charCodeAt(0)); }
function xor(bytes: Uint8Array, key: Uint8Array) { return bytes.map((value, index) => value ^ key[index % key.length]); }

async function deriveKey(salt: Uint8Array<ArrayBuffer>) {
  const material = await crypto.subtle.importKey("raw", encoder.encode(secret), "PBKDF2", false, ["deriveKey"]);
  return crypto.subtle.deriveKey({ name: "PBKDF2", salt, iterations: 100000, hash: "SHA-256" }, material, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
}

export async function loadPublicKey() {
  const stored = localStorage.getItem(publicKeyStorage) || "";
  if (!stored) return "";
  try {
    if (stored.startsWith(xorPrefix)) return decoder.decode(xor(base64ToBytes(stored.slice(xorPrefix.length)), encoder.encode(secret)));
    if (!stored.startsWith(encPrefix)) return stored;
    const parts = stored.split(":");
    if (parts.length !== 5 || !crypto.subtle) return "";
    const salt = base64ToBytes(parts[2]) as Uint8Array<ArrayBuffer>;
    const iv = base64ToBytes(parts[3]) as Uint8Array<ArrayBuffer>;
    const plain = await crypto.subtle.decrypt({ name: "AES-GCM", iv }, await deriveKey(salt), base64ToBytes(parts[4]));
    return decoder.decode(plain);
  } catch {
    localStorage.removeItem(publicKeyStorage);
    return "";
  }
}

export async function savePublicKey(value: string) {
  const key = value.trim();
  if (!key) { localStorage.removeItem(publicKeyStorage); return; }
  if (!crypto.subtle) {
    localStorage.setItem(publicKeyStorage, xorPrefix + bytesToBase64(xor(encoder.encode(key), encoder.encode(secret))));
    return;
  }
  const salt = crypto.getRandomValues(new Uint8Array(16));
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const cipher = new Uint8Array(await crypto.subtle.encrypt({ name: "AES-GCM", iv }, await deriveKey(salt), encoder.encode(key)));
  localStorage.setItem(publicKeyStorage, `${encPrefix}${bytesToBase64(salt)}:${bytesToBase64(iv)}:${bytesToBase64(cipher)}`);
}

export function clearPublicKey() { localStorage.removeItem(publicKeyStorage); }
