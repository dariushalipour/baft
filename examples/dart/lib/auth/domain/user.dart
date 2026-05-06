// lib/auth/domain/user.dart — Auth domain entities

class BaseEntity {
  final String id;
  final DateTime createdAt;
  final DateTime updatedAt;

  BaseEntity({
    required this.id,
    required this.createdAt,
    required this.updatedAt,
  });
}

class User extends BaseEntity {
  final String email;
  final String name;
  final UserRole role;
  final UserProfile? profile;

  User({
    required String id,
    required DateTime createdAt,
    required DateTime updatedAt,
    required this.email,
    required this.name,
    required this.role,
    this.profile,
  }) : super(id: id, createdAt: createdAt, updatedAt: updatedAt);
}

enum UserRole { admin, member, viewer }

class UserProfile {
  final String? avatarUrl;
  final String? bio;
  final String timezone;

  UserProfile({this.avatarUrl, this.bio, required this.timezone});
}

abstract class UserRepository {
  Future<User?> findById(String id);
  Future<User?> findByEmail(String email);
  Future<User> create(User user);
}
