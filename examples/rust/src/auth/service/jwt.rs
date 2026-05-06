use crate::auth::domain::{Token, TokenRepository, User, UserRepository};
use crate::auth::domain::error::AuthError;
use std::time::{Duration, SystemTime};

pub struct JwtService {
    user_repo: Box<dyn UserRepository>,
    token_repo: Box<dyn TokenRepository>,
}

impl JwtService {
    pub fn new(user_repo: Box<dyn UserRepository>, token_repo: Box<dyn TokenRepository>) -> Self {
        Self { user_repo, token_repo }
    }

    pub fn generate_token(&self, user: &User) -> Result<String, AuthError> {
        let token = Token {
            value: format!("fake-jwt-token-for-{}", user.base.id),
            user_id: user.base.id.clone(),
            role: format!("{:?}", user.role),
            expires_at: SystemTime::now() + Duration::from_secs(3600),
        };
        self.token_repo.save(&token)?;
        Ok(token.value)
    }

    pub fn validate_token(&self, token_value: &str) -> Result<Option<User>, AuthError> {
        let token = self.token_repo.find_by_value(token_value)?;
        let token = match token {
            Some(t) => t,
            None => return Ok(None),
        };
        let user = self.user_repo.find_by_id(&token.user_id)?;
        Ok(user)
    }

    pub fn require_auth(&self, token_value: Option<&str>) -> Result<User, AuthError> {
        let token_value = token_value.ok_or_else(AuthError::unauthorized)?;
        let user = self.validate_token(token_value)?
            .ok_or_else(|| AuthError::not_found("token"))?;
        Ok(user)
    }
}
