import 'dart:convert';
import 'dart:io';

class Logger {
  void debug(String message, {Map<String, dynamic>? context}) {
    _log('debug', message, context);
  }

  void info(String message, {Map<String, dynamic>? context}) {
    _log('info', message, context);
  }

  void warn(String message, {Map<String, dynamic>? context}) {
    _log('warn', message, context);
  }

  void error(String message, {Map<String, dynamic>? context}) {
    _log('error', message, context);
  }

  void _log(String level, String message, Map<String, dynamic>? context) {
    final map = <String, dynamic>{
      'level': level,
      'message': message,
      'timestamp': DateTime.now().toIso8601String(),
    };
    if (context != null) map['context'] = context;
    final line = const JsonEncoder.withIndent('  ').convert(map);
    if (level == 'error' || level == 'warn') {
      stderr.writeln(line);
    } else {
      stdout.writeln(line);
    }
  }
}
