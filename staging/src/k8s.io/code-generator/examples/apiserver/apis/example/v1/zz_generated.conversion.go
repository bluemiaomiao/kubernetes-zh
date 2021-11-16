//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.

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

// Code generated by conversion-gen. DO NOT EDIT.

package v1

import (
	unsafe "unsafe"

	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
	example "k8s.io/code-generator/examples/apiserver/apis/example"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*TestType)(nil), (*example.TestType)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_TestType_To_example_TestType(a.(*TestType), b.(*example.TestType), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*example.TestType)(nil), (*TestType)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_example_TestType_To_v1_TestType(a.(*example.TestType), b.(*TestType), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*TestTypeList)(nil), (*example.TestTypeList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_TestTypeList_To_example_TestTypeList(a.(*TestTypeList), b.(*example.TestTypeList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*example.TestTypeList)(nil), (*TestTypeList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_example_TestTypeList_To_v1_TestTypeList(a.(*example.TestTypeList), b.(*TestTypeList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*TestTypeStatus)(nil), (*example.TestTypeStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_TestTypeStatus_To_example_TestTypeStatus(a.(*TestTypeStatus), b.(*example.TestTypeStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*example.TestTypeStatus)(nil), (*TestTypeStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_example_TestTypeStatus_To_v1_TestTypeStatus(a.(*example.TestTypeStatus), b.(*TestTypeStatus), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1_TestType_To_example_TestType(in *TestType, out *example.TestType, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1_TestTypeStatus_To_example_TestTypeStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1_TestType_To_example_TestType is an autogenerated conversion function.
func Convert_v1_TestType_To_example_TestType(in *TestType, out *example.TestType, s conversion.Scope) error {
	return autoConvert_v1_TestType_To_example_TestType(in, out, s)
}

func autoConvert_example_TestType_To_v1_TestType(in *example.TestType, out *TestType, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_example_TestTypeStatus_To_v1_TestTypeStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_example_TestType_To_v1_TestType is an autogenerated conversion function.
func Convert_example_TestType_To_v1_TestType(in *example.TestType, out *TestType, s conversion.Scope) error {
	return autoConvert_example_TestType_To_v1_TestType(in, out, s)
}

func autoConvert_v1_TestTypeList_To_example_TestTypeList(in *TestTypeList, out *example.TestTypeList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	out.Items = *(*[]example.TestType)(unsafe.Pointer(&in.Items))
	return nil
}

// Convert_v1_TestTypeList_To_example_TestTypeList is an autogenerated conversion function.
func Convert_v1_TestTypeList_To_example_TestTypeList(in *TestTypeList, out *example.TestTypeList, s conversion.Scope) error {
	return autoConvert_v1_TestTypeList_To_example_TestTypeList(in, out, s)
}

func autoConvert_example_TestTypeList_To_v1_TestTypeList(in *example.TestTypeList, out *TestTypeList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	out.Items = *(*[]TestType)(unsafe.Pointer(&in.Items))
	return nil
}

// Convert_example_TestTypeList_To_v1_TestTypeList is an autogenerated conversion function.
func Convert_example_TestTypeList_To_v1_TestTypeList(in *example.TestTypeList, out *TestTypeList, s conversion.Scope) error {
	return autoConvert_example_TestTypeList_To_v1_TestTypeList(in, out, s)
}

func autoConvert_v1_TestTypeStatus_To_example_TestTypeStatus(in *TestTypeStatus, out *example.TestTypeStatus, s conversion.Scope) error {
	out.Blah = in.Blah
	return nil
}

// Convert_v1_TestTypeStatus_To_example_TestTypeStatus is an autogenerated conversion function.
func Convert_v1_TestTypeStatus_To_example_TestTypeStatus(in *TestTypeStatus, out *example.TestTypeStatus, s conversion.Scope) error {
	return autoConvert_v1_TestTypeStatus_To_example_TestTypeStatus(in, out, s)
}

func autoConvert_example_TestTypeStatus_To_v1_TestTypeStatus(in *example.TestTypeStatus, out *TestTypeStatus, s conversion.Scope) error {
	out.Blah = in.Blah
	return nil
}

// Convert_example_TestTypeStatus_To_v1_TestTypeStatus is an autogenerated conversion function.
func Convert_example_TestTypeStatus_To_v1_TestTypeStatus(in *example.TestTypeStatus, out *TestTypeStatus, s conversion.Scope) error {
	return autoConvert_example_TestTypeStatus_To_v1_TestTypeStatus(in, out, s)
}
