# encoding: utf-8
import os
import tempfile
import unittest


class YamlParserTest(unittest.TestCase):
    """config.YamlParser 加载与读写测试（safe_load 路径）"""

    def _write(self, content):
        fd, path = tempfile.mkstemp(suffix='.yaml')
        with os.fdopen(fd, 'w', encoding='utf-8') as fh:
            fh.write(content)
        self.addCleanup(os.remove, path)
        return path

    def test_load_and_get(self):
        from config import YamlParser
        path = self._write("server:\n  port: 8000\ndb_configs:\n  url: mongodb://127.0.0.1:27017\n")
        conf = YamlParser(path)
        self.assertEqual(conf.server['port'], 8000)
        self.assertEqual(conf.db_configs['url'], 'mongodb://127.0.0.1:27017')
        self.assertIsNone(conf.get('not_exist'))

    def test_missing_key_returns_none(self):
        from config import YamlParser
        path = self._write("server:\n  port: 1\n")
        conf = YamlParser(path)
        self.assertIsNone(conf.get('nothing'))


if __name__ == '__main__':
    unittest.main()
