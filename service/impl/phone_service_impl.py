# encoding: utf-8
"""
--------------------------------------
@describe 手机服务接口 对应表device
@version: 1.0
@project: CloudControl
@file: phone_service_impl.py
@author: yuanlang
@time: 2019-03-27 10:25
---------------------------------------
"""
import json
import time

import aiohttp

from common.logger import logger
from service.phone_service import PhoneService


class PhoneServiceImpl(PhoneService):

    def __init__(self):
        from database.motor_helper import motor
        self.motor = motor

    @staticmethod
    async def fetch_agent_info(host):
        """拉取手机 atx-agent 的 info；改用 aiohttp，避免阻塞事件循环"""
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(f"http://{host}:7912/info", timeout=aiohttp.ClientTimeout(total=3)) as resp:
                    if resp.status != 200:
                        return {}
                    return json.loads(await resp.text())
        except Exception as e:
            logger.error(e)
            return {}

    async def on_connected(self, identifier, host):
        logger.debug(identifier + " on_connected")
        save = await self.fetch_agent_info(host)
        current_time = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(time.time()))
        item = {"update_time": current_time, "ip": host, "present": True, "udid": identifier,
                "port": 7912, "using": False, "is_server": False}
        save.update(item)
        await self.motor.upsert(identifier, save)

    async def re_connected(self, identifier, host):
        logger.debug(identifier + " re_connected")
        save = await self.fetch_agent_info(host)
        current_time = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(time.time()))
        item = {"update_time": current_time, "ip": host, "present": True, "udid": identifier,
                "port": 7912, "using": False}
        save.update(item)
        await self.motor.upsert(identifier, save)

    async def offline_connected(self, identifier):
        logger.debug(identifier + " offline_connected")
        current_time = time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(time.time()))
        item = {"update_time": current_time, "present": False}
        await self.motor.update(identifier, item)

    async def update_filed(self, identifier, item: dict):
        """更新字段"""
        await self.motor.upsert(identifier, item)

    async def query_info_by_udid(self, udid):
        """
        query_info_by_udid
        :param udid: 手机唯一标示
        :return: 设备信息，不存在时返回 None
        """
        return await self.motor.find_by_udid(udid)

    async def query_device_list(self):
        """
        query_device_list
        :return:
        """
        return await self.motor.find_device_list()

    async def query_device_list_by_present(self):
        """
        query_device_list_by_present
        :return:
        """
        return await self.motor.query_device_list_by_present()


phone_service = PhoneServiceImpl()
