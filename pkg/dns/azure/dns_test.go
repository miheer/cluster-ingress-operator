package azure_test

import (
	"testing"

	"github.com/pkg/errors"

	configv1 "github.com/openshift/api/config/v1"

	iov1 "github.com/openshift/api/operatoringress/v1"
	"github.com/openshift/cluster-ingress-operator/pkg/dns"
	"github.com/openshift/cluster-ingress-operator/pkg/dns/azure"
	"github.com/openshift/cluster-ingress-operator/pkg/dns/azure/client"
)

func fakeManager(fc *client.FakeDNSClient) (dns.Provider, error) {
	cfg := azure.Config{}
	mgr, err := azure.NewFakeProvider(cfg, fc)
	if err != nil {
		errors.New("failed to create manager")
	}
	return mgr, nil
}

func TestEnsureDNS(t *testing.T) {
	c := client.Config{}
	fc, _ := client.NewFake(c)
	mgr, err := fakeManager(fc)
	if err != nil {
		t.Error("failed to setup the manager under test")
	}
	rg := "test-rg"
	zone := "dnszone.io"
	ARecordName := "subdomain"
	record := iov1.DNSRecord{
		Spec: iov1.DNSRecordSpec{
			DNSName:    "subdomain.dnszone.io.",
			RecordType: iov1.ARecordType,
			Targets:    []string{"55.11.22.33"},
			RecordTTL:  120,
		},
	}
	dnsZone := configv1.DNSZone{
		ID: "/subscriptions/E540B02D-5CCE-4D47-A13B-EB05A19D696E/resourceGroups/test-rg/providers/Microsoft.Network/dnszones/dnszone.io",
	}
	err = mgr.Ensure(&record, dnsZone)
	if err != nil {
		t.Fatalf("unexpected error from Ensure: %v", err)
	}

	recordedCall, _ := fc.RecordedCall(rg, zone, ARecordName)

	if recordedCall != "PUT" {
		t.Fatalf("expected the dns client Put' func to be called, but found %q instead", recordedCall)
	}
}

func TestDeleteDNS(t *testing.T) {
	c := client.Config{}
	fc, _ := client.NewFake(c)
	mgr, err := fakeManager(fc)
	if err != nil {
		t.Fatalf("failed to setup the manager under test: %v", err)
	}
	rg := "test-rg"
	zone := "dnszone.io"
	ARecordName := "subdomain"
	record := iov1.DNSRecord{
		Spec: iov1.DNSRecordSpec{
			DNSName:    "subdomain.dnszone.io.",
			RecordType: iov1.ARecordType,
			Targets:    []string{"55.11.22.33"},
		},
	}
	dnsZone := configv1.DNSZone{
		ID: "/subscriptions/E540B02D-5CCE-4D47-A13B-EB05A19D696E/resourceGroups/test-rg/providers/Microsoft.Network/dnszones/dnszone.io",
	}
	err = mgr.Delete(&record, dnsZone)
	if err != nil {
		t.Fatalf("unexpected error from Delete: %v", err)
	}

	recordedCall, _ := fc.RecordedCall(rg, zone, ARecordName)

	if recordedCall != "DELETE" {
		t.Fatalf("expected the dns client 'Delete' func to be called, but found %q instead", recordedCall)
	}
}

func TestDeleteDNSWithMultipleAddresses(t *testing.T) {
	c := client.Config{}
	fc, _ := client.NewFake(c)
	mgr, err := fakeManager(fc)
	if err != nil {
		t.Fatalf("failed to setup the manager under test: %v", err)
	}
	rg := "test-rg"
	zone := "dnszone.io"
	ARecordName := "subdomain"
	record := iov1.DNSRecord{
		Spec: iov1.DNSRecordSpec{
			DNSName:    "subdomain.dnszone.io.",
			RecordType: iov1.ARecordType,
			Targets:    []string{"55.11.22.33", "55.11.22.34"},
		},
	}
	dnsZone := configv1.DNSZone{
		ID: "/subscriptions/E540B02D-5CCE-4D47-A13B-EB05A19D696E/resourceGroups/test-rg/providers/Microsoft.Network/dnszones/dnszone.io",
	}

	err = mgr.Delete(&record, dnsZone)
	if err != nil {
		t.Fatalf("unexpected error from Delete: %v", err)
	}

	recordedCall, _ := fc.RecordedCall(rg, zone, ARecordName)

	if recordedCall != "DELETE" {
		t.Fatalf("expected the dns client 'Delete' func to be called, but found %q instead", recordedCall)
	}
}

func TestEnsureDNSWithMultipleAddresses(t *testing.T) {
	c := client.Config{}
	fc, _ := client.NewFake(c)
	mgr, err := fakeManager(fc)
	if err != nil {
		t.Fatalf("failed to setup the manager under test: %v", err)
	}
	rg := "test-rg"
	zone := "dnszone.io"
	ARecordName := "subdomain"
	record := iov1.DNSRecord{
		Spec: iov1.DNSRecordSpec{
			DNSName:    "subdomain.dnszone.io.",
			RecordType: iov1.ARecordType,
			Targets:    []string{"55.11.22.33", "55.11.22.34"},
			RecordTTL:  120,
		},
	}
	dnsZone := configv1.DNSZone{
		ID: "/subscriptions/E540B02D-5CCE-4D47-A13B-EB05A19D696E/resourceGroups/test-rg/providers/Microsoft.Network/dnszones/dnszone.io",
	}

	err = mgr.Ensure(&record, dnsZone)
	if err != nil {
		t.Fatalf("unexpected error from Ensure: %v", err)
	}

	recordedCall, _ := fc.RecordedCall(rg, zone, ARecordName)

	if recordedCall != "PUT" {
		t.Fatalf("expected the dns client 'Put' func to be called, but found %q instead", recordedCall)
	}
}
