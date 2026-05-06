import 'package:uuid/uuid.dart';

String generateUuid() => const Uuid().v4();

bool validateUuid(String id) => Uuid.isValidUUID(fromString: id);
