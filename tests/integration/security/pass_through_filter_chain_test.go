// Copyright 2019 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package security

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/test/echo/common/response"
	epb "istio.io/istio/pkg/test/echo/proto"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/echoboot"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/resource/environment"
	"istio.io/istio/pkg/test/util/file"
	"istio.io/istio/pkg/test/util/retry"
	"istio.io/istio/pkg/test/util/tmpl"
)

// TestPassThroughFilterChain tests the authN and authZ policy on the pass through filter chain.
func TestPassThroughFilterChain(t *testing.T) {
	framework.
		NewTest(t).
		RequiresEnvironment(environment.Kube).
		Run(func(ctx framework.TestContext) {
			ns := namespace.NewOrFail(t, ctx, namespace.Config{
				Prefix: "pass-through-filter-chain",
				Inject: true,
			})
			args := map[string]string{
				"Namespace": ns.Name(),
			}
			policies := tmpl.EvaluateAllOrFail(t, args,
				file.AsStringOrFail(t, "testdata/pass-through-filter-chain.yaml.tmpl"))
			g.ApplyConfigOrFail(t, ns, policies...)
			defer g.DeleteConfigOrFail(t, ns, policies...)

			newEchoConfig := func(service string) echo.Config {
				return echo.Config{
					Service:   service,
					Namespace: ns,
					Subsets:   []echo.SubsetConfig{{}},
					Galley:    g,
					Pilot:     p,
					Ports: []echo.Port{
						{
							Name:     "grpc",
							Protocol: protocol.GRPC,
						},
					},
					// The port 8085/8086 will be defined only in the workload and not in the k8s service.
					WorkloadOnlyPorts: []int{
						8085,
						8086,
					},
				}
			}

			var x, a, b, c, d echo.Instance
			echoboot.NewBuilderOrFail(t, ctx).
				With(&x, newEchoConfig("x")).
				With(&a, newEchoConfig("a")).
				With(&b, newEchoConfig("b")).
				With(&c, newEchoConfig("c")).
				With(&d, newEchoConfig("d")).
				BuildOrFail(t)

			cases := []struct {
				target echo.Instance
				port   int
				want   bool
			}{
				// For workload a, there is no authN/authZ policy.
				// All requests should success, this is to verify the pass through filter chain and
				// the workload ports are working correctly.
				{
					target: a,
					port:   8085,
					want:   true,
				},
				{
					target: a,
					port:   8086,
					want:   true,
				},

				// For workload b, there is only authZ policy that allows access to port 8085.
				// Only request to port 8085 should be allowed.
				{
					target: b,
					port:   8085,
					want:   true,
				},
				{
					target: b,
					port:   8086,
					want:   false,
				},

				// For workload c, there is only authN policy that enables mTLS (Strict).
				// The request should be denied because the x is always using plain text.
				{
					target: c,
					port:   8085,
					want:   false,
				},
				{
					target: c,
					port:   8086,
					want:   false,
				},

				// For workload d, there is only authN policy that enables mTLS (Permissive).
				// The request should be allowed because the x is always using plain text.
				{
					target: d,
					port:   8085,
					want:   true,
				},
				{
					target: d,
					port:   8086,
					want:   true,
				},
			}

			for _, tc := range cases {
				name := fmt.Sprintf("x->%s:%d[%t]", tc.target.Config().Service, tc.port, tc.want)
				from := getWorkload(a, t)
				// The request should be handled by the pass through filter chain.
				host := fmt.Sprintf("%s:%d", getWorkload(tc.target, t).Address(), tc.port)
				request := &epb.ForwardEchoRequest{
					Url:   fmt.Sprintf("http://%s/", host),
					Count: 1,
					Headers: []*epb.Header{
						{
							Key:   "Host",
							Value: host,
						},
					},
				}
				t.Run(name, func(t *testing.T) {
					retry.UntilSuccessOrFail(t, func() error {
						responses, err := from.ForwardEcho(context.TODO(), request)
						if tc.want {
							if err != nil {
								return fmt.Errorf("want allow but got error: %v", err)
							}
							if len(responses) < 1 {
								return fmt.Errorf("received no responses from request to %s", host)
							}
							if response.StatusCodeOK != responses[0].Code {
								return fmt.Errorf("want status %s but got %s", response.StatusCodeOK, responses[0].Code)
							}
						} else if err == nil || !strings.Contains(err.Error(), "EOF") {
							// The request should always be rejected in TCP level because we currently do not support
							// HTTP filters in pass through filter chains.
							return fmt.Errorf("want error EOF but got: %v", err)
						}
						return nil
					}, retry.Delay(250*time.Millisecond), retry.Timeout(30*time.Second))
				})
			}
		})
}

func getWorkload(instance echo.Instance, t *testing.T) echo.Workload {
	workloads, err := instance.Workloads()
	if err != nil {
		t.Fatalf(fmt.Sprintf("failed to get Subsets: %v", err))
	}
	if len(workloads) < 1 {
		t.Fatalf("want at least 1 workload but found 0")
	}
	return workloads[0]
}
