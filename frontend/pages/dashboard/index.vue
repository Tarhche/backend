<template>
      <div class="animsition">
        <div class="page-wrapper">

          <!-- WELCOME-->
          <section class="welcome2 p-t-40 p-b-55">
            <DashboardHomeWelcome/>
          </section>
          <!-- END WELCOME-->

          <!-- PAGE CONTENT-->
          <div class="page-container3">
            <section class="alert-wrap p-t-70 p-b-70">
              <div class="container">
                <!-- ALERT-->
                                  <DashboardHomeAlert />
                <!-- END ALERT-->
              </div>
            </section>
            <section>
              <div class="container">
                <div class="row">
                  <div class="col-xl-3">
                    <!-- MENU SIDEBAR-->
                    <aside class="menu-sidebar3 js-spe-sidebar">
                      <dashboard-home-side-bar-menu/>
                    </aside>
                    <!-- END MENU SIDEBAR-->
                  </div>
                  <div class="col-xl-9">
                    <!-- PAGE CONTENT-->
                    <div class="page-content">
                      <div class="row">
                        <div class="col-lg-8">
                        </div>
                      </div>
                      <div class="row">
                        <div class="col-md-12">
                          <!-- DATA TABLE-->
                          <DashboardHomeDataTable :data="data"/>
                          <!-- END DATA TABLE                  -->
                        </div>
                      </div>
                      <div class="row">
                        <div class="col-lg-6">
                          <DashboardHomeTasksCalenders/>
                        </div>
                        <div class="col-lg-6">
                          <DashboardHomeChat/>
                        </div>
                      </div>
                    </div>
                    <!-- END PAGE CONTENT-->
                  </div>
                </div>
              </div>
            </section>
          </div>
          <!-- END PAGE CONTENT  -->

        </div>
      </div>
</template>
<script setup>
import Dashboard from "~/layouts/dashboard.vue";

definePageMeta({
  layout: "dashboard"
})
const data = ref({
  dataTable: [
    {
      date: "2018-09-29 05:57",
      device: "Mobile",
      deviceName: "iPhone X 64Gb Grey",
      success: "Processed",
      price: "$999.00"
    },
    {
      date: "2018-09-28 01:22",
      device: "Mobile",
      deviceName: "Samsung S8 Black",
      success: "Processed",
      price: "$756.00"
    },
    {
      date: "2018-09-27 02:12",
      device: "Game",
      deviceName: "Game Console Controller",
      success: "Denied",
      price: "$22.00"
    },
    {
      date: "2018-09-26 23:06",
      device: "Mobile",
      deviceName: "iPhone X 256Gb Black",
      success: "Denied",
      price: "$1199.00"
    },
    {
      date: "2018-09-25 19:03",
      device: "Accessories",
      deviceName: "USB 3.0 Cable",
      success: "Processed",
      price: "$10.00"
    },
    {
      date: "2018-09-29 05:57",
      device: "Accesories",
      deviceName: "Smartwatch 4.0 LTE Wifi",
      success: "Denied",
      price: "$199.00"
    },
    {
      date: "2018-09-24 19:10",
      device: "Camera",
      deviceName: "Camera C430W 4k",
      success: "Processed",
      price: "$699.00"
    },
    {
      date: "2018-09-22 00:43",
      device: "Computer",
      deviceName: "Macbook Pro Retina 2017",
      success: "Processed",
      price: "$10.00"
    },
  ]
})

</script>
