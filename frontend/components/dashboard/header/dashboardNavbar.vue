<script setup lang="ts">
import "~/assets/css/dashboradMain.css"
import img from "~/assets/images/icon/avatar-01.jpg"

</script>

<template>
  <header class="header-desktop4">
    <div class="container">
      <div class="header4-wrap flex ">
        <div class="header__logo">
          <nuxt-link to="#">
            <span>طرح چه</span>
          </nuxt-link>
        </div>
        <div class="header__tool">
          <div class="header-button-item has-noti js-item-menu">
            <i class="zmdi zmdi-notifications"></i>
            <div class="notifi-dropdown js-dropdown">
              <div class="notifi__title">
                <p>You have 3 Notifications</p>
              </div>
              <div class="notifi__item">
                <div class="bg-c1 img-cir img-40">
                  <i class="zmdi zmdi-email-open"></i>
                </div>
                <div class="content">
                  <p>You got a email notification</p>
                  <span class="date">April 12, 2018 06:50</span>
                </div>
              </div>
              <div class="notifi__item">
                <div class="bg-c2 img-cir img-40">
                  <i class="zmdi zmdi-account-box"></i>
                </div>
                <div class="content">
                  <p>Your account has been blocked</p>
                  <span class="date">April 12, 2018 06:50</span>
                </div>
              </div>
              <div class="notifi__item">
                <div class="bg-c3 img-cir img-40">
                  <i class="zmdi zmdi-file-text"></i>
                </div>
                <div class="content">
                  <p>You got a new file</p>
                  <span class="date">April 12, 2018 06:50</span>
                </div>
              </div>
              <div class="notifi__footer">
                <nuxt-link to="#">All notifications</nuxt-link>
              </div>
            </div>
          </div>
          <div class="header-button-item js-item-menu">
            <i class="zmdi zmdi-settings"></i>
            <div class="setting-dropdown js-dropdown">
              <div class="account-dropdown__body">
                <div class="account-dropdown__item">
                  <nuxt-link to="#">
                    <i class="zmdi zmdi-account"></i>Account</nuxt-link>
                </div>
                <div class="account-dropdown__item">
                  <nuxt-link to="#">
                    <i class="zmdi zmdi-settings"></i>Setting</nuxt-link>
                </div>
                <div class="account-dropdown__item">
                  <nuxt-link to="#">
                    <i class="zmdi zmdi-money-box"></i>Billing</nuxt-link>
                </div>
              </div>
              <div class="account-dropdown__body">
                <div class="account-dropdown__item">
                  <nuxt-link to="#">
                    <i class="zmdi zmdi-globe"></i>Language</nuxt-link>
                </div>
                <div class="account-dropdown__item">
                  <nuxt-link to="#">
                    <i class="zmdi zmdi-pin"></i>Location</nuxt-link>
                </div>
                <div class="account-dropdown__item">
                  <nuxt-link to="#">
                    <i class="zmdi zmdi-email"></i>Email</nuxt-link>
                </div>
                <div class="account-dropdown__item">
                  <nuxt-link to="#">
                    <i class="fa-solid fa-bell"></i>Notifications</nuxt-link>
                </div>
              </div>
            </div>
          </div>
          <div class="account-wrap">
            <div class="account-item account-item--style2 clearfix js-item-menu">
              <div class="image">
                <img :src="img" alt="John Doe" />
              </div>
              <div class="content">
                <nuxt-link class="js-acc-btn" to="#">owner</nuxt-link>
              </div>
              <div class="account-dropdown js-dropdown">
                <div class="info clearfix">
                  <div class="image">
                    <nuxt-link to="#">
                      <img :src="img" alt="John Doe" />
                    </nuxt-link>
                  </div>
                  <div class="content">
                    <h5 class="name">
                      <nuxt-link to="#">john doe</nuxt-link>
                    </h5>
                    <span class="email">johndoe@example.com</span>
                  </div>
                </div>
                <div class="account-dropdown__body">
                  <div class="account-dropdown__item">
                    <nuxt-link to="#">
                      <i class="zmdi zmdi-account"></i>Account</nuxt-link>
                  </div>
                  <div class="account-dropdown__item">
                    <nuxt-link to="#">
                      <i class="zmdi zmdi-settings"></i>Setting</nuxt-link>
                  </div>
                  <div class="account-dropdown__item">
                    <nuxt-link to="#">
                      <i class="zmdi zmdi-money-box"></i>Billing</nuxt-link>
                  </div>
                </div>
                <div class="account-dropdown__footer">
                  <nuxt-link to="#">
                    <i class="zmdi zmdi-power"></i>Logout</nuxt-link>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </header>
</template>

<style scoped>

</style>