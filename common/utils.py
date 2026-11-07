#!/usr/local/bin/python
# coding:utf-8

"""
@author: Liubing
@software: PyCharm
@file: utils.py
@time: 2019-04-17 11:37
@describe:
"""

import socket


def get_host_ip():
    """
    查询本机ip地址；无外网路由时回退 127.0.0.1，保证服务离线也能启动
    :return: ip
    """
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.settimeout(2)
        s.connect(('8.8.8.8', 80))
        ip = s.getsockname()[0]
    except Exception:
        ip = '127.0.0.1'
    finally:
        s.close()

    return ip
