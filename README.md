# fox

Fox is a volume backup manager for Kubernetes.

When you create a pvcBackup custom resource,
the custom controller starts backup of a specified PVC.
Fox is backed by [rdiff-backup](https://rdiff-backup.net/), so it supports differential backup.

See [an example](https://github.com/peng225/fox/blob/main/config/samples/fox_v1alpha1_pvcbackup.yaml) to use pvcBackup.

**Caution**

- This project is created for me(@peng225/@peng225_pub) to practice writing a custom controller. It has an apparent security problem, so do not use it in any production environments.
- The following problems are left unresolved.
  - How to restore backup data?
  - When deleted a pvcBackup resource, how to handle older backups than the deleted one? By the restriction of rdiff-backup, I may have to delete those older backups.

  I may try to resolve these problems when I get enough time.
