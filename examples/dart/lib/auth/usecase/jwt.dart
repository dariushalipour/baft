// lib/auth/usecase/jwt.dart — JWT token generation and validation

import '../domain/user.dart';
import '../domain/errors.dart';

class JwtService {
  Future<String> generateToken(User user) async {
    return 'fake-jwt-token-for-${user.id}';
  }

  Future<User?> validateToken(String token) async {
    // In a real app, this would decode and verify the JWT
    return null;
  }

  Future<User> requireAuth(String? token) async {
    if (token == null || token.isEmpty) {
      throw DomainError(code: 'unauthorized', message: 'Authorization header required');
    }
    final user = await validateToken(token);
    if (user == null) {
      throw NotFoundError('token');
    }
    return user;
  }
}
