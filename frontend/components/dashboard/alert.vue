<template>
  <div class="alert au-alert-success alert-dismissible fade show au-alert au-alert--70per" role="alert">
    <i class="zmdi zmdi-check-circle"></i>
    <span class="content"><slot /></span>
    <button class="close" type="button" data-dismiss="alert" aria-label="Close">
      <span aria-hidden="true">
          <i class="zmdi zmdi-close-circle"></i>
      </span>
    </button>
  </div>
</template>
