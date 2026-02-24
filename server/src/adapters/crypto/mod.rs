use crate::ports::CryptoService;
use ed25519_dalek::{SigningKey, Signature, VerifyingKey, Verifier};
use rand::rngs::OsRng;

pub struct Ed25519CryptoService;

impl CryptoService for Ed25519CryptoService {
    fn generate_keypair(&self) -> (String, String) {
        let mut csprng = OsRng;
        let signing_key = SigningKey::generate(&mut csprng);
        let verifying_key = signing_key.verifying_key();
        
        (
            hex::encode(verifying_key.to_bytes()),
            hex::encode(signing_key.to_bytes()),
        )
    }

    fn verify_signature(&self, public_key: &str, message: &[u8], signature: &str) -> bool {
        let pub_key_bytes = match hex::decode(public_key) {
            Ok(b) => b,
            Err(_) => return false,
        };
        
        let sig_bytes = match hex::decode(signature) {
            Ok(b) => b,
            Err(_) => return false,
        };

        if pub_key_bytes.len() != 32 || sig_bytes.len() != 64 {
            return false;
        }

        let mut pub_key_array = [0u8; 32];
        pub_key_array.copy_from_slice(&pub_key_bytes);

        let mut sig_array = [0u8; 64];
        sig_array.copy_from_slice(&sig_bytes);

        let verifying_key = match VerifyingKey::from_bytes(&pub_key_array) {
            Ok(k) => k,
            Err(_) => return false,
        };

        let sig = Signature::from_bytes(&sig_array);

        verifying_key.verify(message, &sig).is_ok()
    }
}
