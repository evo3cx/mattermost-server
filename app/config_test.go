// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package app

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/store/sqlstore"
	"github.com/mattermost/mattermost-server/utils"
)

func TestConfigListener(t *testing.T) {
	th := Setup().InitBasic()
	defer th.TearDown()

	originalSiteName := th.App.Config().TeamSettings.SiteName
	th.App.UpdateConfig(func(cfg *model.Config) {
		cfg.TeamSettings.SiteName = "test123"
	})

	listenerCalled := false
	listener := func(oldConfig *model.Config, newConfig *model.Config) {
		if listenerCalled {
			t.Fatal("listener called twice")
		}

		if oldConfig.TeamSettings.SiteName != "test123" {
			t.Fatal("old config contains incorrect site name")
		} else if newConfig.TeamSettings.SiteName != originalSiteName {
			t.Fatal("new config contains incorrect site name")
		}

		listenerCalled = true
	}
	listenerId := th.App.AddConfigListener(listener)
	defer th.App.RemoveConfigListener(listenerId)

	listener2Called := false
	listener2 := func(oldConfig *model.Config, newConfig *model.Config) {
		if listener2Called {
			t.Fatal("listener2 called twice")
		}

		listener2Called = true
	}
	listener2Id := th.App.AddConfigListener(listener2)
	defer th.App.RemoveConfigListener(listener2Id)

	th.App.ReloadConfig()

	if !listenerCalled {
		t.Fatal("listener should've been called")
	} else if !listener2Called {
		t.Fatal("listener 2 should've been called")
	}
}

func TestAsymmetricSigningKey(t *testing.T) {
	th := Setup().InitBasic()
	defer th.TearDown()
	assert.NotNil(t, th.App.AsymmetricSigningKey())
	assert.NotEmpty(t, th.App.ClientConfig()["AsymmetricSigningPublicKey"])
}

func TestClientConfigWithComputed(t *testing.T) {
	th := Setup().InitBasic()
	defer th.TearDown()

	config := th.App.ClientConfigWithComputed()
	if _, ok := config["NoAccounts"]; !ok {
		t.Fatal("expected NoAccounts in returned config")
	}
	if _, ok := config["MaxPostSize"]; !ok {
		t.Fatal("expected MaxPostSize in returned config")
	}
}

func TestEnsureInstallationDate(t *testing.T) {
	th := Setup()
	defer th.TearDown()

	tt := []struct {
		Name                     string
		PrevInstallationDate     *int64
		UsersCreationDates       []int64
		ExpectedInstallationDate *int64
	}{
		{
			Name:                     "New installation: no users, no installation date",
			PrevInstallationDate:     nil,
			UsersCreationDates:       nil,
			ExpectedInstallationDate: model.NewInt64(utils.MillisFromTime(time.Now())),
		},
		{
			Name:                     "Old installation: users, no installation date",
			PrevInstallationDate:     nil,
			UsersCreationDates:       []int64{10000000000, 30000000000, 20000000000},
			ExpectedInstallationDate: model.NewInt64(10000000000),
		},
		{
			Name:                     "New installation, second run: no users, installation date",
			PrevInstallationDate:     model.NewInt64(80000000000),
			UsersCreationDates:       []int64{10000000000, 30000000000, 20000000000},
			ExpectedInstallationDate: model.NewInt64(80000000000),
		},
		{
			Name:                     "Old installation already updated: users, installation date",
			PrevInstallationDate:     model.NewInt64(90000000000),
			UsersCreationDates:       []int64{10000000000, 30000000000, 20000000000},
			ExpectedInstallationDate: model.NewInt64(90000000000),
		},
	}

	for _, tc := range tt {
		t.Run(tc.Name, func(t *testing.T) {
			sqlStore := th.App.Srv.Store.User().(*sqlstore.SqlUserStore)
			sqlStore.GetMaster().Exec("DELETE FROM Users")

			var users []*model.User
			for _, createAt := range tc.UsersCreationDates {
				user := th.CreateUser()
				user.CreateAt = createAt
				sqlStore.GetMaster().Exec("UPDATE Users SET CreateAt = :CreateAt WHERE Id = :UserId", map[string]interface{}{"CreateAt": createAt, "UserId": user.Id})
				users = append(users, user)
			}

			if tc.PrevInstallationDate == nil {
				<-th.App.Srv.Store.System().PermanentDeleteByName(model.SYSTEM_INSTALLATION_DATE_KEY)
			} else {
				<-th.App.Srv.Store.System().SaveOrUpdate(&model.System{
					Name:  model.SYSTEM_INSTALLATION_DATE_KEY,
					Value: strconv.FormatInt(*tc.PrevInstallationDate, 10),
				})
			}

			err := th.App.ensureInstallationDate()

			if tc.ExpectedInstallationDate == nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				result := <-th.App.Srv.Store.System().GetByName(model.SYSTEM_INSTALLATION_DATE_KEY)
				assert.Nil(t, result.Err)
				data, _ := result.Data.(*model.System)
				value, _ := strconv.ParseInt(data.Value, 10, 64)
				assert.True(t, *tc.ExpectedInstallationDate <= value && *tc.ExpectedInstallationDate+1000 >= value)
			}

			sqlStore.GetMaster().Exec("DELETE FROM Users")
		})
	}
}
