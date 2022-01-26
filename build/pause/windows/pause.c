/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

#include <windows.h>
#include <stdio.h>

BOOL WINAPI CtrlHandler(DWORD fdwCtrlType)
{
	switch (fdwCtrlType)
	{
	case CTRL_C_EVENT:
		fprintf(stderr, "收到信号, 正在关闭...\n");
		exit(0);

	case CTRL_BREAK_EVENT:
		fprintf(stderr, "收到信号, 正在关闭...\n");
		exit(0);

	default:
		return FALSE;
	}
}

int main(void)
{
	if (SetConsoleCtrlHandler(CtrlHandler, TRUE))
	{
		Sleep(INFINITE);
	}
	else
	{
		printf("\n错误: 无法设置控制器\n");
		return 1;
	}
	return 0;
}
