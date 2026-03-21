"""
ed25519 cryptographic operations for the memro protocol.
Uses PyNaCl (libsodium bindings).
"""
import time
from typing import Tuple

try:
    import nacl.signing
    import nacl.encoding
except ImportError:
    raise ImportError(
        "PyNaCl is required: pip install pynacl"
    )


def generate_keypair() -> Tuple[str, str]:
    """
    Generate a new ed25519 keypair.
    Returns (public_key_hex, private_key_hex).
    The public key hex is the agent_id.
    """
    signing_key = nacl.signing.SigningKey.generate()
    verify_key = signing_key.verify_key

    private_key_hex = signing_key.encode(encoder=nacl.encoding.HexEncoder).decode()
    public_key_hex = verify_key.encode(encoder=nacl.encoding.HexEncoder).decode()

    return public_key_hex, private_key_hex


def sign_body(private_key_hex: str, body: bytes) -> dict:
    """
    Sign timestamp + body with ed25519 private key.
    Returns headers dict ready to merge into a request.
    """
    try:
        private_key_bytes = bytes.fromhex(private_key_hex)
    except ValueError:
        raise ValueError("Invalid private key: must be a 64-character hexadecimal string.")
        
    signing_key = nacl.signing.SigningKey(private_key_bytes)

    timestamp = str(int(time.time()))

    # Robust Payload: TIMESTAMP + BODY
    payload = timestamp.encode() + body

    # Sign the payload
    signed = signing_key.sign(payload)
    signature_hex = signed.signature.hex()

    return {
        "X-Signature": signature_hex,
        "X-Timestamp": timestamp,
    }
