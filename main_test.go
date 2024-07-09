package main

import (
	"reflect"
	"testing"
)

func Test_healthyServers(t *testing.T) {
	type args struct {
		s              []Servers
		unhealthyHosts map[Servers]bool
	}
	tests := []struct {
		name string
		args args
		want []Servers
	}{
		// TODO: Add test cases.
		{
			name: "Test healthyServers",
			args: args{
				s: []Servers{
					{
						URL: "http://localhost:8080",
					},
					{
						URL: "http://localhost:8081",
					},
				},
				unhealthyHosts: map[Servers]bool{
					{
						URL: "http://localhost:8080",
					}: true,
				},
			},
			want: []Servers{
				{
					URL: "http://localhost:8081",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := healthyServers(tt.args.s, tt.args.unhealthyHosts); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("healthyServers() = %v, want %v", got, tt.want)
			}
		})
	}
}
