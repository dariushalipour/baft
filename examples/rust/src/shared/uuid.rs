use uuid::Uuid;

pub fn generate_uuid() -> String {
    Uuid::new_v4().to_string()
}

pub fn validate_uuid(id: &str) -> bool {
    Uuid::parse_str(id).is_ok()
}
