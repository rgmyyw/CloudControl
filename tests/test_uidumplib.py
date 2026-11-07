# encoding: utf-8
import unittest

from common import uidumplib


class ParseUiautomatorNodeTest(unittest.TestCase):
    """uidumplib.parse_uiautomator_node 纯逻辑测试"""

    def test_android_node_fields(self):
        import xml.dom.minidom
        xml_text = uidumplib.sample_android_page_xml
        node = xml.dom.minidom.parseString(xml_text).getElementsByTagName('node')[0]
        ks = uidumplib.parse_uiautomator_node(node)
        # bounds 应被别名成 rect 且解析为四元组
        self.assertIn('rect', ks)
        self.assertEqual(ks['rect'], {'x': 0, 'y': 0, 'width': 720, 'height': 1280})
        self.assertNotIn('bounds', ks)
        # 文本与布尔解析
        self.assertEqual(ks['className'], 'android.widget.FrameLayout')
        self.assertFalse(ks['clickable'])
        self.assertTrue(ks['enabled'])

    def test_text_node(self):
        import xml.dom.minidom
        xml_text = uidumplib.sample_android_page_xml
        doc = xml.dom.minidom.parseString(xml_text)
        # 找一个带 text 的节点（梦幻西游）
        target = None
        for n in doc.getElementsByTagName('node'):
            if n.getAttribute('text') == '梦幻西游':
                target = n
                break
        self.assertIsNotNone(target)
        ks = uidumplib.parse_uiautomator_node(target)
        self.assertEqual(ks['text'], '梦幻西游')
        self.assertEqual(ks['index'], 1)
        self.assertEqual(ks['rect'], {'x': 8, 'y': 66, 'width': 176, 'height': 204})


if __name__ == '__main__':
    unittest.main()
