#!/usr/local/bin/python
# coding:utf-8

"""
@author: Liubing
@software: PyCharm
@file: utils.py
@time: 2019-04-17 11:37
@describe:
"""

import os
import socket


def get_host_ip():
    """
    查询本机ip地址；无外网路由时回退 127.0.0.1，保证服务离线也能启动。
    容器部署时容器内探测到的是内网 IP，可用 CC_HOST_IP 环境变量指定对外地址
    :return: ip
    """
    if os.environ.get("CC_HOST_IP"):
        return os.environ["CC_HOST_IP"]
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
